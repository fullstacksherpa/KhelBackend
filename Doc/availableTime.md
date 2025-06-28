## Booking Availability: Implementation and Data Flow

### Overview

This document summarizes the end-to-end implementation of the hourly availability feature for venues, covering both frontend and backend components, the data flow between them, potential flaws, and a sample output.

---

## Frontend Implementation

### 1. `useAvailableTimes` Hook

```ts
export interface AvailableTimesVariables {
  venueID: number | string;
  /** Full RFC3339 timestamp in Kathmandu timezone. */
  date: string; // e.g. `2025-06-28T00:00:00+05:45`
}

export const useAvailableTimes = createQuery<
  AvailableTimesResponse,
  AvailableTimesVariables,
  AxiosError<APIError>
>({
  queryKey: ["venues", "available-times"],
  fetcher: ({ venueID, date }) =>
    client.get(`/venues/${venueID}/available-times`, { params: { date } }).then((res) => res.data),
  staleTime: 40_000,
  refetchInterval: 30_000,
});
```

- Fetches availability slots by passing `date` as full ISO timestamp in Kathmandu time.
- Caching and refetch intervals configured for responsiveness.

### 2. `AvailableTimesScreen` Component

```tsx
const dates = useMemo(() => generateDatesArray(10), []);
const [selectedDate, setSelectedDate] = useState(dates[0].fullDate);

const { data, isLoading, error } = useAvailableTimes({
  variables: { venueID: venueId.toString(), date: selectedDate },
});
```

- Generates 10-day window using `generateDatesArray` (returns full ISO in +05:45).
- Uses `FlatList` to render `TimeSlotCard` for each `HourlySlot`.

---

## Backend Implementation

### 1. `availableTimesHandler`

```go
dateStr := r.URL.Query().Get("date")
date, err := time.ParseInLocation(time.RFC3339, dateStr, loc)
loc, _ := time.LoadLocation("Asia/Kathmandu")
dateInKtm := date.In(loc)
dayOfWeek := strings.ToLower(dateInKtm.Weekday().String())
pricingSlots := store.GetPricingSlots(ctx, venueID, dayOfWeek)
bookings := store.GetBookingsForDate(ctx, venueID, date)
// convert bookings to Kathmandu intervals
// generate hourly buckets and check overlaps
```

- Parses full timestamp, converts to Kathmandu to determine `dayOfWeek`.

### 2. Data Access Layer

#### a) `GetPricingSlots`

- Queries `venue_pricing` for given `venueID` and `day_of_week`.
- Returns list of `PricingSlot` with local-times in UTC `time.Time` fields.

#### b) `GetBookingsForDate`

```go
loc, _ := time.LoadLocation("Asia/Kathmandu")
localDate := date.In(loc)
startOfDayLocal := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0,0,0,0,loc)
endOfDayLocal := startOfDayLocal.Add(24*time.Hour)
// convert to UTC
startUTC := startOfDayLocal.UTC()
endUTC := endOfDayLocal.UTC()
// SELECT ... WHERE start_time < $1 AND end_time > $2
```

- Computes local day bounds in Kathmandu, converts to UTC for querying `timestamptz` columns.

---

## Data Flow

```text
1. User Interaction:
   → User selects a date via DateSelector
   → selectedDate becomes e.g. 2025-06-28T00:00:00+05:45

2. Frontend Request:
   → useAvailableTimes sends:
     GET /venues/{id}/available-times?date=2025-06-28T00:00:00+05:45

3. Handler Parsing:
   → availableTimesHandler parses `dateStr`
   → converts to `dateInKtm`
   → determines `dayOfWeek`

4. Pricing Lookup:
   → Calls GetPricingSlots(venueID, dayOfWeek)
   → returns daily pricing windows

5. Booking Lookup:
   → Calls GetBookingsForDate(ctx, venueID, date)
   → computes local day bounds and queries bookings

6. Slot Generation:
   → Handler breaks pricing windows into 1‑hour buckets
   → Filters out past hours
   → Marks availability by checking overlaps with existing bookings

7. Response:
   → Returns JSON array of HourlySlot objects
   → Format: { start_time, end_time, price_per_hour, available }
   → All timestamps in Kathmandu-local time
```

---

## Server Log Sample

```log
⏰ dateStr: 2025-06-29T00:00:00+05:45
⏰ date after Parse: 2025-06-29 00:00:00 +0545 +0545
⏰ date in Ktm: 2025-06-29 00:00:00 +0545 +0545
⏰ day of week in ktm: sunday
```

- ✅ You're receiving the date string in proper RFC3339 format with Kathmandu offset.
- ✅ You're parsing it using `time.ParseInLocation`, attaching the named location `Asia/Kathmandu`.
- ✅ You're converting and computing the local day correctly.
- ✅ You're querying pricing and bookings properly.
- ✅ You're generating hourly time slot output accurately.

> **Note:** `+0545 +0545` appears twice because Go's internal formatting shows both the numeric offset and the fixed location (if not a named zone).

You can verify the named location with:

```go
fmt.Printf("Time: %s | Location: %s\n", date.Format(time.RFC3339), date.Location())
```

---
