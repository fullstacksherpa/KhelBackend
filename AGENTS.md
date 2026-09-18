This file describes the structure of the Khel Go backend so future agents and developers can make changes without losing the shape of the system.

## Project Purpose

Khel is a Go HTTP API for a sports platform with venue discovery, venue owner operations, bookings, games, user accounts, push notifications, ads, app reviews, venue onboarding requests, and a merchant store with carts, orders, featured products, and Nepali payment gateways.

The backend is a single Go module named `khel`. The public API is mounted under `/v1`.

## Main Runtime Flow

1. `cmd/api/main.go` loads environment configuration.
2. `internal/db.New` creates a `pgxpool.Pool`.
3. `internal/domain/storage.NewContainer` wires all domain repositories using the database pool.
4. `cmd/api/main.go` creates external clients:
   - Cloudinary for image storage.
   - Mailtrap mailer for email delivery.
   - Expo adapter for push notifications.
   - JWT authenticator for access and refresh tokens.
   - Payment manager with eSewa and Khalti gateways.
   - Fixed-window rate limiters.
5. `cmd/api/api.go` builds the Chi router, global middleware, route groups, role checks, and debug/docs endpoints.
6. `application.run` starts the HTTP server and handles graceful shutdown.
7. `cmd/api/background.go` starts periodic game completion work using the app context.

## Top-Level Structure

- `cmd/api`: HTTP application, route mounting, middleware, request/response DTOs, and handlers.
- `cmd/migrate/migrations`: SQL migrations for PostgreSQL schema, indexes, enums, and triggers.
- `internal/db`: PostgreSQL connection pool creation.
- `internal/infra/dbx`: Shared `Querier` interface implemented by both `pgxpool.Pool` and `pgx.Tx`.
- `internal/domain`: Domain packages and PostgreSQL repositories.
- `internal/domain/storage`: Repository container and transaction-scoped repository bundles.
- `internal/auth`: JWT creation and validation interfaces.
- `internal/mailer`: Mail client interface and Mailtrap implementation.
- `internal/notifications`: Push notification sender and domain notification helpers.
- `internal/payments`: Payment gateway interfaces and eSewa/Khalti adapters.
- `internal/params`: Shared pagination/query parameter helpers.
- `internal/ratelimiter`: Fixed-window limiter implementation.
- `docs`: Generated Swagger files. Do not edit generated files by hand.
- `Doc`: Human-written notes and operational documentation.
- `.github/workflows`: CI checks for build, vet, staticcheck, dependencies, and tests.

## Application Object

Most HTTP handlers are methods on:

```go
type application struct {
    config        config
    store         *storage.Container
    logger        *zap.SugaredLogger
    cld           *cloudinary.Cloudinary
    mailer        mailer.Client
    authenticator auth.Authenticator
    rateLimiter   ratelimiter.Limiter
    venueRequestLimiter ratelimiter.Limiter
    push          *notifications.ExpoAdapter
    hashID        *hashids.HashID
    payments      *payments.PaymentManager
}
```

Handlers should use this object instead of creating their own infrastructure clients. New dependencies should be wired in `main.go`, added to `application`, and used through small interfaces where practical.

## HTTP Layer

`cmd/api/api.go` owns routing. The API uses:

- Chi router.
- Chi middleware for request ID, slash stripping, real IP, logging, recovery, and timeouts.
- CORS.
- JWT authentication middleware.
- Optional auth middleware for public endpoints that can enrich responses when a user is logged in.
- Role middleware backed by `internal/domain/accesscontrol`.
- Owner/admin middleware for venue and game permissions.

Handler files are grouped by product area:

- `auth.go`, `auth_web.go`: registration, login, refresh, logout, password reset, cookie auth.
- `users.go`: profile, profile image, follow/unfollow, current user, admin user view.
- `venues.go`: venue CRUD, photos, search, favorites, status.
- `facilities.go`, `facility_*`: facility CRUD, facility photos, pricing, facility bookings.
- `booking.go`: venue bookings, pricing, owner booking queues, booking state changes.
- `games.go`, `gameQA.go`, `shortlist_games.go`: games, join requests, assistants, Q&A, shortlist.
- `inventory.go`: venue inventory and game checkout item billing.
- `products.go`, `product_images.go`, `product_variants.go`: store catalog, brands, categories, products, variants, images.
- `sales.go`, `payment.go`, `admin_orders.go`, `admin_payments.go`: carts, checkout, orders, payments, gateway redirects.
- `featured.go`: featured collections and featured items.
- `ads.go`: ad CRUD, public ad impressions/clicks, analytics.
- `venue_requests.go`: public venue onboarding requests and admin approval/rejection.
- `venue_customers_handlers.go`, `venue_earnings.go`: venue owner analytics.
- `admin_roles.go`, `admin_dashboard.go`: superadmin operations.
- `pushTokens.go`: Expo push token storage and cleanup.
- `appReview.go`, `review.go`: app and venue reviews.
- `cloudinary.go`, `images.go`: shared upload helpers.
- `json.go`, `errors.go`, `middleware.go`: common HTTP primitives.

## Domain Packages

Each domain package under `internal/domain` owns its types, repository interface, and PostgreSQL implementation. Most repositories are created in `storage.NewContainer`.

### Identity And Access

- `users`: user profile data, passwords, activation, refresh token storage, reset token storage, admin user listing.
- `accesscontrol`: roles and user-role mapping. Used by `RequireRoleMiddleware`.
- `followers`: follow and unfollow relationships between users.

### Venues And Facilities

- `venues`: venue records, ownership checks, search, favorites, photos, status, owner lookup.
- `facilities`: facility records inside a venue, default facility rules, facility image URLs.
- `bookings`: venue/facility pricing slots, booking creation, owner queues, booking state transitions, user booking history.
- `venuereview`: venue reviews, ownership checks, review stats.
- `venuerequest`: public venue onboarding request workflow.
- `venuecustomers`: venue customer summaries and customer detail analytics.
- `venueearnings`: venue earnings calculations over bookings and inventory items.
- `inventory`: venue inventory items, game billing items, active game list, billing summary.

### Games

- `games`: game creation, join requests, admin/assistant checks, players, cancellation, upcoming games, shortlisted games, periodic completion.
- `gameqa`: game questions and replies.

### Store And Payments

- `products`: brands, categories, category tree, products, variants, product images, catalog cards, search, full-text search, best offers.
- `carts`: active cart, cart items, checkout locking, abandoned cart marking, conversion after payment.
- `orders`: order number generation, order snapshots from carts, order items, status updates, admin and user order views.
- `paymentsrepo`: payment rows, provider refs, payment status transitions, payment logs.
- `featured`: featured collections/items and cache refresh for store home surfaces.
- `payments` package outside `domain`: gateway abstraction and adapters for eSewa and Khalti.

### Engagement And Operations

- `ads`: ad CRUD, active ad listing, impressions, clicks, analytics.
- `appreviews`: app review submission and admin listing.
- `pushtokens`: Expo push token save/remove/bulk-remove/prune and user token lookup.
- `admindashboard`: superadmin overview metrics.
- `adminview`: helper rows for admin user overview.

## Important Domain Connections

### Auth

`auth.go` and `auth_web.go` use `users.Store` to load users and refresh tokens. JWTs are generated by `internal/auth`. Role checks for merchant/admin endpoints use `accesscontrol.Store`.

### Venue Ownership

Venue owner routes are protected by `AuthTokenMiddleware` and `IsOwnerMiddleware`. The middleware asks `venues.Store.IsOwner` before allowing nested venue operations such as facility, pricing, booking, inventory, earnings, and customer routes.

### Booking Flow

Bookings connect:

- `venues` for ownership and venue metadata.
- `facilities` for facility-specific bookings and pricing.
- `bookings` for pricing slots, availability, booking rows, and status transitions.
- `inventory` when a venue owner adds billable items to a booked game.
- `pushtokens` and `notifications` for booking/game notifications.

### Game Flow

Games connect:

- `games` for game rows, players, admins, assistants, join requests, and shortlist.
- `bookings` when games are created from booked venue time.
- `gameqa` for questions and replies.
- `notifications` and `pushtokens` for join request and game updates.

### Store Checkout Flow

Checkout is the most sensitive flow:

1. `sales.go` loads the user's active cart.
2. `orders.CreateFromCart` locks the cart row with `FOR UPDATE`.
3. It snapshots current catalog prices and featured discounts into `orders` and `order_items`.
4. For online payment, it moves the cart to `checkout_pending` and creates a `payments` row.
5. Gateway initiation happens outside the DB transaction.
6. Gateway redirect/verify handlers call the gateway API outside the DB transaction.
7. `storage.Container.WithSalesTx` is used to atomically mark payment/order state and convert or unlock the cart.

Use `WithSalesTx` for any checkout/payment operation that updates more than one of carts, orders, payments, and payment logs.

### Featured Products

`featured` links store home collections to product/product variant rows. `orders.CreateFromCart` also reads `featured_items` and `featured_collections` to compute checkout-time discounts. Changing featured deal semantics can affect both storefront display and checkout totals.

### Images

Image handlers upload to Cloudinary first and then persist URLs in domain repositories. Venue/facility/product/user/ad upload logic is currently spread across handler files and `cloudinary.go`.

## Repository Pattern

Most domain packages expose a `Store` interface and a `Repository` implementation. Keep SQL in repositories, not in handlers.

For transaction-aware repositories, prefer accepting `dbx.Querier` instead of only `*pgxpool.Pool`. This allows the same repository to run against either a pool or a `pgx.Tx`.

Example:

```go
type Repository struct {
    q dbx.Querier
}
```

`carts`, `orders`, and `paymentsrepo` already follow this pattern for checkout transactions.

## Coding Rules For This Project

- Keep handlers focused on HTTP concerns: parse input, authorize, call domain store/service, write response.
- Keep SQL and transaction decisions in domain repositories or storage-level unit-of-work helpers.
- Use `r.Context()` or a child context for request work. Avoid `context.Background()` inside handlers and external API helpers unless the work intentionally outlives the request.
- Do not ignore errors inside transactions. Return the error so the transaction rolls back.
- Use structured logging with zap `Infow`, `Warnw`, `Errorw`, or `Debugw` when passing key/value fields.
- Use `http.MaxBytesReader` before parsing multipart uploads.
- Validate user-controlled payloads before repository calls. Prefer typed payload structs over `map[string]any`.
- Use parameterized SQL values. Dynamic SQL is acceptable only when field names are selected from a strict allow-list.
- Keep payment gateway verification as the source of truth. Redirect params and client payloads are not trusted.
- Do not edit generated Swagger files directly. Regenerate them with the project command.
- Do not commit `.env*` files or secrets.
- Keep migrations append-only. Do not rewrite applied migrations unless you are intentionally rebuilding a non-shared development database.

## Common Commands

```bash
go test ./...
go test -race ./...
go vet ./...
go mod verify
staticcheck ./...
make dev-up
make dev-down 1
make staging-up
make prod-up
make prod-down 1 CONFIRM=yes
```

The local `.env.*` files are used by the `Makefile` for migration targets. The application loads `.env.development` automatically only when `DOCKER` is not set and `APP_ENV` is `development`.

## Testing Expectations

At the time this guide was written, the repository compiles but has no `_test.go` files. New work should add focused tests around:

- Auth token creation, refresh, and middleware behavior.
- Repository methods with transaction-sensitive behavior.
- Checkout state transitions.
- Gateway verification decision logic.
- Booking availability and pricing.
- Upload validation helpers.
- Rate limiter behavior.

For database-heavy code, prefer repository tests against a disposable PostgreSQL database with migrations applied. For handler code, use `httptest` and fake stores where possible.

## Improvement Backlog

High-priority improvements:

- Add real test coverage. Current `go test ./...` reports no test files.
- Split very large handler and repository files into smaller service/use-case units.
- Normalize environment naming. The code currently uses both `APP_ENV=prod` and `ENV=production`.
- Remove activation tokens from registration responses and raise the minimum password policy.
- Fix shared error logging helpers to use zap structured logging methods.
- Ensure all multipart upload handlers use `http.MaxBytesReader`.
- Replace request-path `context.Background()` calls with request contexts.
- Return transaction errors instead of ignoring them in payment failure branches.
- Make JWT constructor/config explicit and use configured token durations.
- Make the general rate limiter use normalized client IPs and avoid per-client sleeping goroutines.
- Remove unrelated scratch files such as root-level `test.go`.
