package main

import (
	"time"
	"os"
	"fmt"
	"encoding/gob"
)

// Every 5 minutes save the current sessions to file \\
func periodicTestSave() {
	rate := time.Minute * 5
	throttle := time.Tick(rate)
	for {
		<- throttle
		saveTestsToFile()
	}
}

// Loads sessions from sessions file \\
func loadTestsFromFile() {
	f, err := os.Open(workDir + "/tests.db")
	if err != nil {
		fmt.Println(err)
	}
	decoder := gob.NewDecoder(f)
	SpeedTestsLock.Lock()
	decoder.Decode(&SpeedTests)
	SpeedTestsLock.Unlock()
	f.Close()
}

// Saves sessions to sessions file \\
func saveTestsToFile() {
	f, err := os.Create(workDir + "/tests.db")
	if err != nil {
		fmt.Println(err)
	}
	encoder := gob.NewEncoder(f)
	SpeedTestsLock.Lock()
	encoder.Encode(&SpeedTests)
	SpeedTestsLock.Unlock()
	f.Close()
}