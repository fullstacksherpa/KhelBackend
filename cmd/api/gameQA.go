package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"khel/internal/notifications"
	"khel/internal/store"

	"github.com/go-chi/chi/v5"
)

type QuestionPayload struct {
	Question string `json:"question" validate:"required,max=120"`
}

type ReplyPayload struct {
	Reply string `json:"reply" validate:"required,max=120"`
}

// @Summary		Create a game question
// @Description	Create a new question for a game
// @Tags			Questions
// @Accept			json
// @Produce		json
// @Param			gameID	path		int				true	"Game ID"
// @Param			payload	body		QuestionPayload	true	"Question payload"
// @Success		201		{object}	store.Question
// @Failure		400		{object}	error
// @Failure		401		{object}	error
// @Failure		404		{object}	error
// @Security		ApiKeyAuth
// @Router			/games/{gameID}/questions [post]
func (app *application) createQuestionHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "gameID"), 10, 64)
	fmt.Printf("successful parse gameID %v", gameID)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid game ID"))
		return
	}

	var payload QuestionPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := getUserFromContext(r)
	fmt.Printf("login userID is %v", user.ID)

	question := &store.Question{
		GameID:   gameID,
		UserID:   user.ID,
		Question: payload.Question,
	}

	if err := app.store.GameQA.CreateQuestion(r.Context(), question); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	notifications.CallAsync(func(ctx context.Context) error {
		return notifications.NotifyGameQuestionToAdmin(ctx, app.push, app.store, gameID, user.FirstName)
	}, "SendingCreateGameQuestion")

	app.jsonResponse(w, http.StatusCreated, question)
}

// @Summary		Get game questions
// @Description	Get all questions for a game
// @Tags			Questions
// @Accept			json
// @Produce		json
// @Param			gameID	path		int	true	"Game ID"
// @Success		200		{array}		store.Question
// @Failure		400		{object}	error
// @Security		ApiKeyAuth
// @Router			/games/{gameID}/questions [get]
func (app *application) getGameQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "gameID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid game ID"))
		return
	}

	questions, err := app.store.GameQA.GetQuestionsByGame(r.Context(), gameID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, questions)
}

// @Summary		Create a reply
// @Description	Create a reply to a question (admin only)
// @Tags			Questions
// @Accept			json
// @Produce		json
// @Param			gameID		path		int				true	"Game ID"
// @Param			questionID	path		int				true	"Question ID"
// @Param			payload		body		ReplyPayload	true	"Reply payload"
// @Success		201			{object}	store.Reply
// @Failure		400			{object}	error
// @Failure		401			{object}	error
// @Security		ApiKeyAuth
// @Router			/games/{gameID}/questions/{questionID}/replies [post]
func (app *application) createReplyHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "gameID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid gameID"))
		return
	}

	questionID, err := strconv.ParseInt(chi.URLParam(r, "questionID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid question ID"))
		return
	}

	var payload ReplyPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := getUserFromContext(r)

	reply := &store.Reply{
		QuestionID: questionID,
		AdminID:    user.ID,
		Reply:      payload.Reply,
	}

	if err := app.store.GameQA.CreateReply(r.Context(), reply); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	notifications.CallAsync(func(ctx context.Context) error {
		return notifications.SendQuestionReply(ctx, app.push, app.store, questionID, gameID)
	}, "SendingQuestionReplyToUser")

	app.jsonResponse(w, http.StatusCreated, reply)
}

// @Summary		Delete a question
// @Description	Delete a question (only by author)
// @Tags			Questions
// @Accept			json
// @Produce		json
// @Param			gameID		path	int	true	"Game ID"
// @Param			questionID	path	int	true	"Question ID"
// @Success		204
// @Failure		400	{object}	error
// @Failure		401	{object}	error
// @Failure		404	{object}	error
// @Security		ApiKeyAuth
// @Router			/games/{gameID}/questions/{questionID} [delete]
func (app *application) deleteQuestionHandler(w http.ResponseWriter, r *http.Request) {
	questionID, err := strconv.ParseInt(chi.URLParam(r, "questionID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid question ID"))
		return
	}

	user := getUserFromContext(r)

	if err := app.store.GameQA.DeleteQuestion(r.Context(), questionID, user.ID); err != nil {
		if errors.Is(err, store.ErrQuestionNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Get all Q&A for a game
// @Description	Get all questions with replies for a game
// @Tags			Questions
// @Accept			json
// @Produce		json
// @Param			gameID	path		int	true	"Game ID"
// @Success		200		{array}		store.QuestionWithReplies
// @Failure		400		{object}	error
// @Router			/games/{gameID}/qa [get]
func (app *application) getGameQAHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "gameID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid game ID"))
		return
	}

	qa, err := app.store.GameQA.GetQuestionsWithReplies(r.Context(), gameID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, qa)
}
