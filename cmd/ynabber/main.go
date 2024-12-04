package main

import (
	"fmt"
	"github.com/carlmjohnson/versioninfo"
	"github.com/kelseyhightower/envconfig"
	"github.com/martinohansen/ynabber"
	"github.com/martinohansen/ynabber/reader/nordigen"
	"github.com/martinohansen/ynabber/writer/json"
	"github.com/martinohansen/ynabber/writer/ynab"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func setupLogging(debug bool) {
	programLevel := slog.LevelInfo
	if debug {
		programLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(
		os.Stderr, &slog.HandlerOptions{
			Level: programLevel,
		}))
	slog.SetDefault(logger)
}

func main() {
	// Read config from env
	var cfg ynabber.Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	setupLogging(cfg.Debug)
	slog.Info("starting...", "version", versioninfo.Short())

	ynabber := ynabber.Ynabber{}
	for _, reader := range cfg.Readers {
		switch reader {
		case "nordigen":
			ynabber.Readers = append(ynabber.Readers, nordigen.NewReader(&cfg))
		default:
			log.Fatalf("Unknown reader: %s", reader)
		}
	}
	for _, writer := range cfg.Writers {
		switch writer {
		case "ynab":
			ynabber.Writers = append(ynabber.Writers, ynab.NewWriter(&cfg))
		case "json":
			ynabber.Writers = append(ynabber.Writers, json.Writer{})
		default:
			log.Fatalf("Unknown writer: %s", writer)
		}
	}

	port := fmt.Sprintf(":%d", cfg.Port)
	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		bankID := r.URL.Query().Get("bankID")

		if bankID == "" {
			bankID = cfg.Nordigen.BankID
		}

		err := run(ynabber, bankID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "Run succeeded")
			slog.Info("run succeeded", "in", time.Since(start))
			os.Exit(0)
		}
	})

	slog.Info("Server running on ", "port", port)
	log.Fatal(http.ListenAndServe(port, nil))

}

func run(y ynabber.Ynabber, bankID string) error {
	var transactions []ynabber.Transaction

	// Read transactions from all readers
	for _, reader := range y.Readers {
		t, err := reader.Bulk(bankID)
		if err != nil {
			return fmt.Errorf("reading: %w", err)
		}
		transactions = append(transactions, t...)
	}

	// Write transactions to all writers
	for _, writer := range y.Writers {
		err := writer.Bulk(transactions)
		if err != nil {
			return fmt.Errorf("writing: %w", err)
		}
	}
	return nil
}
