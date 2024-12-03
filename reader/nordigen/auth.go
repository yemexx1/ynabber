package nordigen

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"

	"github.com/frieser/nordigen-go-lib/v2"
)

const RequisitionRedirect = "https://raw.githubusercontent.com/martinohansen/ynabber/main/ok.html"

// requisitionStore returns a clean path to the requisition file
func (r Reader) requisitionStore(bankID string) string {
	// Use BankID or RequisitionFile as filename
	var file string
	if r.Config.Nordigen.RequisitionFile == "" {
		file = bankID
	} else {
		file = r.Config.Nordigen.RequisitionFile
	}

	return path.Clean(fmt.Sprintf("%s/%s.json", r.Config.DataDir, file))
}

// Requisition tries to get requisition from disk, if it fails it will create a
// new and store that one to disk.
func (r Reader) Requisition(bankID string) (nordigen.Requisition, error) {
	requisitionFile, err := os.ReadFile(r.requisitionStore(bankID))
	if errors.Is(err, os.ErrNotExist) {
		log.Print("Requisition is not found")
		return r.createRequisition(bankID)
	} else if err != nil {
		return nordigen.Requisition{}, fmt.Errorf("ReadFile: %w", err)
	}

	var requisition nordigen.Requisition
	err = json.Unmarshal(requisitionFile, &requisition)
	if err != nil {
		log.Print("Failed to parse requisition file")
		return r.createRequisition(bankID)
	}

	switch requisition.Status {
	case "EX":
		// Create a new requisition if expired
		log.Printf("Requisition is expired")
		return r.createRequisition(bankID)
	case "LN":
		// Return requisition if it's still valid
		return requisition, nil
	default:
		// Handle unknown status by recreating requisition
		log.Printf("Unsupported requisition status: %s", requisition.Status)
		return r.createRequisition(bankID)
	}
}

func (r Reader) saveRequisition(requisition nordigen.Requisition, bankID string) error {
	requisitionFile, err := json.Marshal(requisition)
	if err != nil {
		return err
	}

	err = os.WriteFile(r.requisitionStore(bankID), requisitionFile, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (r Reader) createRequisition(bankID string) (nordigen.Requisition, error) {
	if bankID == "" {
		bankID = r.Config.Nordigen.BankID
	}

	requisition, err := r.Client.CreateRequisition(nordigen.Requisition{
		Redirect:      RequisitionRedirect,
		Reference:     strconv.Itoa(int(time.Now().Unix())),
		Agreement:     "",
		InstitutionId: bankID,
	})
	if err != nil {
		return nordigen.Requisition{}, fmt.Errorf("CreateRequisition: %w", err)
	}

	r.requisitionHook(requisition)
	log.Printf("Initiate requisition by going to: %s", requisition.Link)

	// Keep waiting for the user to accept the requisition
	maxTries := 5
	try := 0
	for requisition.Status != "LN" && try < maxTries {
		requisition, err = r.Client.GetRequisition(requisition.Id)
		if err != nil {
			return nordigen.Requisition{}, fmt.Errorf("GetRequisition: %w", err)
		}
		time.Sleep(60 * time.Second)
		try++
	}

	// Store requisition on disk
	err = r.saveRequisition(requisition, bankID)
	if err != nil {
		log.Printf("Failed to write requisition to disk: %s", err)
	}

	return requisition, nil
}

// requisitionHook executes the hook with the status and link as arguments
func (r Reader) requisitionHook(req nordigen.Requisition) {
	if r.Config.Nordigen.RequisitionHook != "" {
		cmd := exec.Command(r.Config.Nordigen.RequisitionHook, req.Status, req.Link)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("failed to run requisition hook: %s, output: %s", err, string(output))
		}
	}
}
