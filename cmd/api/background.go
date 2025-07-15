package main

import (
	"context"
	"time"
)

func (app *application) runMarkCompletedGames() {
	if err := app.store.Games.MarkCompletedGames(); err != nil {
		app.logger.Errorf("Error marking games as completed: %v", err)
	} else {
		app.logger.Infof("Successfully marked games as completed at %s", time.Now().UTC().Format(time.RFC3339))
	}
}

func (app *application) markCompletedGamesEvery30Mins(ctx context.Context) {
	go func() {

		defer func() {
			if r := recover(); r != nil {
				app.logger.Errorf("Recovered from panic in markCompletedGamesEvery30Mins: %v", r)
			}
		}()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		// Run once immediately
		app.runMarkCompletedGames()

		for {
			select {
			case <-ctx.Done():
				app.logger.Info("Stopped markCompletedGamesEvery30Mins due to context cancellation")
				return
			case <-ticker.C:
				app.runMarkCompletedGames()
			}
		}
	}()
}
