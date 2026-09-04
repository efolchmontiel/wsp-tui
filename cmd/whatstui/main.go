package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"github.com/efolchmontiel/wsp-tui/internal/engine"
	"github.com/efolchmontiel/wsp-tui/internal/logging"
	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/paths"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"github.com/efolchmontiel/wsp-tui/internal/syncer"
	"github.com/efolchmontiel/wsp-tui/internal/ui"
	"github.com/efolchmontiel/wsp-tui/internal/version"
)

func main() {
	debug := flag.Bool("debug", false, "enable debug logging to the log file")
	showVersion := flag.Bool("version", false, "print version and exit")
	logout := flag.Bool("logout", false, "logout and delete local WhatsApp session")
	reset := flag.Bool("reset", false, "delete local session database (and related files)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("wsp-tui %s\n", version.Version)
		return
	}

	p, err := paths.Resolve()
	if err != nil {
		fatal(err)
	}
	if err := config.EnsureDefault(p.ConfigFile); err != nil {
		fatal(err)
	}

	logger, logFile, err := logging.Setup(p.LogFile, *debug)
	if err != nil {
		fatal(err)
	}
	defer logFile.Close()

	appStore, err := store.Open(p.AppDB)
	if err != nil {
		fatal(err)
	}
	defer appStore.Close()

	bus := app.NewBus(256)
	syn := syncer.New(appStore, bus, logger)
	eng := engine.New(p, logger, *debug, bus, appStore, syn)
	mediaMgr := media.New(p, appStore, eng.Client)
	eng.SetMedia(mediaMgr)
	syn.SetMedia(mediaMgr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	syn.Start(ctx)
	defer syn.Stop()

	if *logout || *reset {
		if err := eng.Start(ctx); err != nil {
			logger.Warn("engine start for logout/reset", "err", err)
		}
		if *logout {
			if err := eng.Logout(ctx); err != nil {
				fatal(err)
			}
			fmt.Println("Logged out and removed local session.")
		} else {
			if err := eng.Reset(); err != nil {
				fatal(err)
			}
			fmt.Println("Local session reset.")
		}
		eng.Close()
		return
	}

	if err := eng.Start(ctx); err != nil {
		fatal(err)
	}
	defer eng.Close()

	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		logger.Warn("config load", "err", err)
		cfg = config.Default()
	}

	// Local retention: drop messages/media older than configured window (phone untouched).
	go func() {
		keep, enabled := cfg.LocalRetention.Duration()
		stats, err := appStore.MaybePurgeOlderThan(ctx, time.Now(), p.MediaDir, keep, enabled)
		if err != nil {
			logger.Warn("local purge", "err", err)
			return
		}
		if stats.Skipped {
			return
		}
		logger.Info("local purge",
			"retention", cfg.LocalRetention.Label(),
			"messages", stats.Messages, "media", stats.Media, "files", stats.Files)
		if stats.Messages > 0 || stats.Media > 0 || stats.Files > 0 {
			bus.Publish(app.Event{
				Kind: app.EventInfo,
				Message: fmt.Sprintf(
					"Limpieza local (>%s): %d msgs · %d media · %d archivos",
					cfg.LocalRetention.Label(), stats.Messages, stats.Media, stats.Files,
				),
			})
		}
	}()

	if err := ui.Run(bus, eng, appStore, cfg, p.ConfigFile); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "wsp-tui: %v\n", err)
	os.Exit(1)
}
