package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/carlmjohnson/versioninfo"
	"github.com/kelseyhightower/envconfig"
	"github.com/martinohansen/ynabber"
	"github.com/martinohansen/ynabber/reader/nordigen"
	"github.com/martinohansen/ynabber/writer/json"
	"github.com/martinohansen/ynabber/writer/ynab"
)

func main() {
	log.Println("Version:", versioninfo.Short())

	// Read config from env
	var cfg ynabber.Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Check that some values are valid
	cfg.YNAB.Cleared = strings.ToLower(cfg.YNAB.Cleared)
	if cfg.YNAB.Cleared != "cleared" &&
		cfg.YNAB.Cleared != "uncleared" &&
		cfg.YNAB.Cleared != "reconciled" {
		log.Fatal("YNAB_CLEARED must be one of cleared, uncleared or reconciled")
	}

	if cfg.Debug {
		log.Printf("Config: %+v\n", cfg)
	}

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
			ynabber.Writers = append(ynabber.Writers, ynab.Writer{Config: &cfg})
		case "json":
			ynabber.Writers = append(ynabber.Writers, json.Writer{})
		default:
			log.Fatalf("Unknown writer: %s", writer)
		}
	}

	port := fmt.Sprintf(":%d", cfg.Port)
	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		bankID := r.URL.Query().Get("bankID")

		if bankID == "" {
			bankID = cfg.Nordigen.BankID
		}

		go func() {
			err := run(ynabber, bankID)
			if err != nil {
				log.Printf("unable to run sync: %w\n", err)
			}
		}()

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Run succeeded")
	})

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
