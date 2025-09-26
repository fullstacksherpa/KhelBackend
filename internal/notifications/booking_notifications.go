package notifications

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/storage"

	"github.com/9ssi7/exponent"
)

type BookingEvent string

const (
	BookingCreated  BookingEvent = "Created"
	BookingAccepted BookingEvent = "ACCEPTED"
	BookingRejected BookingEvent = "REJECTED"
	BookingCanceled BookingEvent = "CANCELED"
)

func SendBookingNotification(ctx context.Context, push PushSender, store *storage.Container, userID int64, event BookingEvent, bookingID string) error {
	// Fetch tokens for the user
	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{userID})
	if err != nil {
		return err
	}
	tokens := tokensMap[userID]
	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	// Prepare Notification Content
	var title, body string
	switch event {
	case BookingCreated:
		title = "New Booking Request"
		body = "You have a new booking request"
	case BookingAccepted:
		title = "Booking Accepted"
		body = fmt.Sprintf("Your booking (ID: %s) has been confirmed! 🎉", bookingID)
	case BookingRejected:
		title = "Booking Rejected"
		body = fmt.Sprintf("Your booking (ID: %s) has been rejected. ", bookingID)
	case BookingCanceled:
		title = "Booking Cancelled"
		body = fmt.Sprintf("Your booking (ID: %s) has been cancelled", bookingID)
	default:
		title = "Booking Update"
		body = fmt.Sprintf("Your booking (ID: %s) has an update. ", bookingID)
	}

	// Prepare Expo messages
	msgs := make([]*exponent.Message, 0, len(tokens))
	for _, t := range tokens {
		//wrap the string token in exponent.Token
		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			//the data field is what your app receives when a push notification is tapped, and it usually drives deep linking
			Data: map[string]string{
				"type":      "booking",
				"event":     string(event),
				"bookingId": bookingID,
				"screen":    "user-bookings-screen", // / is already at client router.push(`/${data.screen}`);
			},
		}
		msgs = append(msgs, msg)
	}

	_, err = push.Publish(ctx, msgs)
	if err != nil {
		return err
	}
	return nil
}
