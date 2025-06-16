package main

import (
	"time"
)

func (app *application) markCompletedGamesEvery30Mins() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		// Run once immediately
		err := app.store.Games.MarkCompletedGames()
		if err != nil {
			app.logger.Errorf("Error marking games as completed: %v", err)
		} else {
			app.logger.Infof("Successfully marked games as completed at %s", time.Now().Format(time.RFC1123))
		}

		// Then run every 30 minutes
		for range ticker.C {
			err := app.store.Games.MarkCompletedGames()
			if err != nil {
				app.logger.Errorf("Error marking games as completed: %v", err)
			} else {
				app.logger.Infof("Successfully marked games as completed at %s", time.Now().Format(time.RFC1123))
			}
		}
	}()
}
