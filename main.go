package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

var printCount = 0

// Config structure to hold printer and debug configuration
type Config struct {
	RunOnHttps  bool   `json:"https"`
	PrinterName string `json:"printer_name"`
	Debug       bool   `json:"debug"`
	PrintDir    string `json:"print_dir"`
	Port        uint   `json:"port"`
}

// Global variable for the configuration
var config Config

// Function to log if debug is enabled
func debugLog(message string) {
	if config.Debug {
		log.Println(message)
	}
}

// Reads the configuration from a file
func readConfig(filePath string) (Config, error) {
	var conf Config = Config{
		RunOnHttps:  false,
		PrinterName: "",
		Debug:       false,
		PrintDir:    "temp-files",
		Port:        49155,
	}
	file, err := os.ReadFile(filePath)
	if err != nil {
		return conf, err
	}
	err = json.Unmarshal(file, &conf)
	return conf, err
}

// cleanupPrintJobs deletes all .prn files in the specified print directory
func cleanupPrintJobs() {
	debugLog("Starting cleanup of print jobs...")
	files, err := filepath.Glob(filepath.Join(config.PrintDir, "*.prn"))
	if err != nil {
		log.Printf("Failed to glob for .prn files: %v", err)
		return
	}

	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			log.Printf("Failed to delete print job file: %s, error: %v", file, err)
		} else {
			debugLog(fmt.Sprintf("Deleted leftover print job file: %s", file))
		}
	}
	debugLog("Cleanup of print jobs completed.")
}

// Handles incoming print requests
func printHandler(w http.ResponseWriter, req *http.Request) {
	// Set CORS headers for preflight and actual request
	w.Header().Set("Access-Control-Allow-Origin", "*") // Specify the actual origin instead of '*'
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("Access-Control-Max-Age", "3600") // Max age for the preflight request

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if req.Method != "POST" {
		http.Error(w, "Only POST method is accepted", http.StatusMethodNotAllowed)
		return
	}

	debugLog("Received print job request.")

	//if printCount > 20 {
	//	debugLog("Printer exhausted, please restart.")
	//	return
	//}

	// Read the body (expected to be the raw data of the file to print)
	data, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(req.Body)

	// Ensure the print directory exists
	err = os.MkdirAll(config.PrintDir, 0755)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a unique file for the print job
	printJobFileName := filepath.Join(config.PrintDir, fmt.Sprintf("printjob_%d.prn", time.Now().UnixNano()))
	err = os.WriteFile(printJobFileName, data, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	debugLog(fmt.Sprintf("Created print job file: %s", printJobFileName))

	// Schedule file cleanup after 1 hour
	time.AfterFunc(1*time.Hour, func() {
		err := os.Remove(printJobFileName)
		if err != nil {
			log.Printf("Failed to delete print job file: %s, error: %v", printJobFileName, err)
		} else {
			debugLog(fmt.Sprintf("Print job file deleted: %s", printJobFileName))
		}
	})

	// Execute print command based on OS
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if config.PrinterName == "" {
			cmd = exec.Command("print", printJobFileName)
		} else {

			cmd = exec.Command("cmd", "/c", "copy", printJobFileName, config.PrinterName)
		}
	} else {
		if config.PrinterName == "" {
			cmd = exec.Command("lp", printJobFileName)
		} else {
			cmd = exec.Command("lp", "-d", config.PrinterName, printJobFileName)
		}
	}

	printCount = printCount + 1

	err = cmd.Run()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	debugLog(fmt.Sprintf("Print job sent to printer: %s", config.PrinterName))

	_, err = fmt.Fprintf(w, "Print job %s sent successfully.", printJobFileName)
	if err != nil {
		return
	}
}

func ensureDir(dirName string) error {
	err := os.MkdirAll(dirName, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	var err error
	config, err = readConfig("config.json")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	debugLog("Starting server...")
	log.Println("-------------------------------------------------------------")
	log.Println(fmt.Sprintf("Runs on HTTPS: %v", config.RunOnHttps))
	log.Println(fmt.Sprintf("Runs on Port: %d", config.Port))
	if config.PrinterName == "" {
		log.Println(fmt.Sprintf("Print at %s", "Default Printer"))
	} else {
		log.Println(fmt.Sprintf("Print at %s", config.PrinterName))
	}
	log.Println(fmt.Sprintf("Debug: %v", config.Debug))
	log.Println(fmt.Sprintf("Print Directory: %s", config.PrintDir))
	log.Println("-------------------------------------------------------------")
	log.Println("")

	err = ensureDir(config.PrintDir)
	if err != nil {
		log.Fatalf("Error creating directory: %v", err)
	}

	// Perform initial cleanup of any leftover print job files
	cleanupPrintJobs()

	http.HandleFunc("/print", printHandler)
	debugLog("Print handler setup completed.")

	if config.RunOnHttps {
		err := http.ListenAndServeTLS(":49155", "./certificates/server.crt", "./certificates/server.key", nil)
		if err != nil {
			log.Fatal("ListenAndServeTLS: ", err)
		}
	} else {
		log.Fatal(http.ListenAndServe(":49155", nil))
	}
}
