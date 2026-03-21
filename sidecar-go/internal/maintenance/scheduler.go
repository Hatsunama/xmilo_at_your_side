package maintenance

import (
	"context"
	"log"
	"time"
)

func Start(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Println("nightly maintenance tick fired")
			}
		}
	}()
}
