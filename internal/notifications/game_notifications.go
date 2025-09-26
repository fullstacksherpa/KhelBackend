package notifications

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/storage"
	"strconv"
	"time"

	"github.com/9ssi7/exponent"
)

// SendJoinRequestToAdmin - notify game admin(s) that a user requested to join with requesterName
func SendJoinRequestToAdmin(ctx context.Context, push PushSender, store *storage.Container, AdminID int64, gameID int64, requesterName string) error {

	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{AdminID})
	if err != nil {
		return err
	}
	tokens := dedupe(tokensMap[AdminID])
	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	//Prepare expo messages

	msgs := make([]*exponent.Message, 0, len(tokens))
	title := "New game join request"
	body := fmt.Sprintf("%s has sent a join request", requesterName)
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))
	for _, t := range tokens {
		//wrap the string token in exponent.Token to satisfy the type

		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "game_join_request",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				//in client we do router.push(`/${data.screen}`)
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

// SendDeleteJoinRequestToAdmin - notify game admin(s) that a user has deleted join request.
func SendDeleteJoinRequestToAdmin(ctx context.Context, push PushSender, store *storage.Container, gameID int64, requesterName string) error {

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	AdminID, err := store.Games.GetAdminID(ctxTimeout, gameID)

	if err != nil {
		return err
	}

	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{AdminID})
	if err != nil {
		return err
	}

	tokens := dedupe(tokensMap[AdminID])
	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	//Prepare expo messages

	msgs := make([]*exponent.Message, 0, len(tokens))
	title := "Join request withdrawn"
	body := fmt.Sprintf("%s has withdrew join request", requesterName)
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))
	for _, t := range tokens {
		//wrap the string token in exponent.Token to satisfy the type

		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "game_delete_join_request",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				//in client we do router.push(`/${data.screen}`)
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

// SendRejectJoinRequestToUser - notify the requesting user that request was rejected by the game admin
func SendRejectJoinRequestToUser(ctx context.Context, push PushSender, store *storage.Container, userID int64, gameID int64) error {

	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{userID})
	if err != nil {
		return err
	}

	tokens := dedupe(tokensMap[userID])
	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	//Prepare expo messages

	msgs := make([]*exponent.Message, 0, len(tokens))
	title := "Join request rejected"
	body := "Your request to join the game was not accepted"
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))
	for _, t := range tokens {
		//wrap the string token in exponent.Token to satisfy the type

		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "reject_game_join_request",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				//in client we do router.push(`/${data.screen}`)
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

// SendAcceptJoinRequestToUser - notify the requesting user that request was rejected by the game admin
func SendAcceptJoinRequestToUser(ctx context.Context, push PushSender, store *storage.Container, userID int64, gameID int64) error {

	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{userID})
	if err != nil {
		return err
	}
	tokens := dedupe(tokensMap[userID])

	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	//Prepare expo messages

	msgs := make([]*exponent.Message, 0, len(tokens))
	title := "Join request accepted"
	body := "Your game join request was accepted"
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))
	for _, t := range tokens {
		//wrap the string token in exponent.Token to satisfy the type

		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "accept_game_join_request",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				//in client we do router.push(`/${data.screen}`)
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

// SendCancelGameToPlayers - notify all players that a game has been canceled
func SendCancelGameToPlayers(ctx context.Context, push PushSender, store *storage.Container, gameID int64) error {

	// Get all player IDs for the game
	playerIDs, err := store.Games.GetAllGamePlayerIDs(ctx, gameID)
	if err != nil {
		return fmt.Errorf("error getting game players: %w", err)
	}

	if len(playerIDs) == 0 {
		return errors.New("no players found for the game")
	}

	// Get push tokens for all players
	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, playerIDs)
	if err != nil {
		return fmt.Errorf("error getting player tokens: %w", err)
	}

	// Collect all tokens from all players
	allTokens := make([]string, 0)
	for _, tokens := range tokensMap {
		allTokens = append(allTokens, tokens...)
	}

	compactTokens := dedupe(allTokens)

	if len(allTokens) == 0 {
		return errors.New("no push tokens found for any players")
	}

	// Prepare expo messages
	msgs := make([]*exponent.Message, 0, len(allTokens))
	title := "Game Canceled"
	body := "The game you were registered for has been canceled"
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))

	for _, t := range compactTokens {
		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "game_canceled",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				// Client will navigate to games list
			},
		}
		msgs = append(msgs, msg)
	}

	_, err = push.Publish(ctx, msgs)
	if err != nil {
		return fmt.Errorf("error sending cancellation notifications: %w", err)
	}

	return nil
}

// NotifySendGameQuestionToAdmin - notify game admin(s) that a user has send a question
func NotifyGameQuestionToAdmin(ctx context.Context, push PushSender, store *storage.Container, gameID int64, requesterName string) error {

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	AdminID, err := store.Games.GetAdminID(ctxTimeout, gameID)

	if err != nil {
		return err
	}

	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{AdminID})
	if err != nil {
		return err
	}

	tokens := dedupe(tokensMap[AdminID])
	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	//Prepare expo messages

	msgs := make([]*exponent.Message, 0, len(tokens))
	title := "New game Message"
	body := fmt.Sprintf("%s has sent a message", requesterName)
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))
	for _, t := range tokens {
		//wrap the string token in exponent.Token to satisfy the type

		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "game_message_send",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				//in client we do router.push(`/${data.screen}`)
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

// SendQuestionReply - notify question asker that admin has reply
func SendQuestionReply(ctx context.Context, push PushSender, store *storage.Container, questionID int64, gameID int64) error {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	userID, err := store.GameQA.GetUserIDByQuestionID(ctxTimeout, questionID)

	if err != nil {
		return err
	}

	tokensMap, err := store.PushTokens.GetTokensByUserIDs(ctx, []int64{userID})
	if err != nil {
		return err
	}

	tokens := dedupe(tokensMap[userID])
	if len(tokens) == 0 {
		return errors.New("no push tokens")
	}

	//Prepare expo messages

	msgs := make([]*exponent.Message, 0, len(tokens))
	title := "New reply to your question"
	body := "Admin has reply your question"
	screen := fmt.Sprintf("games/%s", strconv.FormatInt(gameID, 10))
	for _, t := range tokens {
		//wrap the string token in exponent.Token to satisfy the type

		token := exponent.Token(t)
		msg := &exponent.Message{
			To:    []*exponent.Token{&token},
			Title: title,
			Body:  body,
			Data: map[string]string{
				"type":    "game_reply_send",
				"game_id": strconv.FormatInt(gameID, 10),
				"screen":  screen,
				//in client we do router.push(`/${data.screen}`)
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
