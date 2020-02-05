package main

import (
	"encoding/json"
	"fmt"
	"github.com/guardian/mediaflipper/common/models"
	"github.com/guardian/mediaflipper/common/results"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

/**
retrieve an object based on the settings passed
*/
func ParseSettings(rawString string) (*models.JobSettings, error) {
	var s models.JobSettings
	marshalErr := json.Unmarshal([]byte(rawString), &s)
	if marshalErr != nil {
		log.Printf("Could not understand passed settings: %s. Offending data was: %s", marshalErr, rawString)
		return nil, marshalErr
	}

	return &s, nil
}

/**
goroutine to monitor the output from the encoding app
*/
func monitorOutput(stdOutChan chan string, stdErrChan chan string, closeChan chan bool) {
	for {
		select {
		case line := <-stdOutChan:
			log.Printf("STDOUT: %s", line)
		case line := <-stdErrChan:
			log.Printf("STDERR: %s", line)
			if strings.HasPrefix(line, "frame=") {
				models.ParseTranscodeProgress(line)
			}
		case <-closeChan:
			log.Print("monitorOutput completed")
			return
		}
	}
}

func RunTranscode(fileName string, settings *models.JobSettings) results.TranscodeResult {
	outFileName := RemoveExtension(fileName) + "_transcoded"

	commandArgs := []string{"-i", fileName}
	commandArgs = append(commandArgs, settings.MarshalToArray()...)
	commandArgs = append(commandArgs, "-y", outFileName)

	startTime := time.Now()

	cmd := exec.Command("/usr/local/bin/ffmpeg", commandArgs...)

	closeChan := make(chan bool)
	stdOutChan, stdErrChan, runErr := RunCommandStreaming(cmd)
	if runErr != nil {
		endTime := time.Now()
		duration := endTime.UnixNano() - startTime.UnixNano()
		log.Printf("Could not execute command: %s", runErr)
		return results.TranscodeResult{
			OutFile:      "",
			TimeTaken:    float64(duration) / 1e9,
			ErrorMessage: fmt.Sprintf("Could not execute command: %s", runErr),
		}
	}

	go monitorOutput(stdOutChan, stdErrChan, closeChan)

	waitErr := cmd.Wait()

	closeChan <- true

	endTime := time.Now()
	duration := endTime.UnixNano() - startTime.UnixNano()
	if waitErr != nil {
		log.Printf("Could not execute command: %s", runErr)
		return results.TranscodeResult{
			OutFile:      "",
			TimeTaken:    float64(duration) / 1e9,
			ErrorMessage: fmt.Sprintf("Could not execute command: %s", runErr),
		}
	}

	_, statErr := os.Stat(outFileName)

	if statErr != nil {
		log.Printf("Transcode completed but could not find output file: %s", statErr)
		return results.TranscodeResult{
			OutFile:      "",
			TimeTaken:    float64(duration) / 1e9,
			ErrorMessage: fmt.Sprintf("Could not execute command: %s", runErr),
		}
	}

	return results.TranscodeResult{
		OutFile:      outFileName,
		TimeTaken:    float64(duration) / 1e9,
		ErrorMessage: "",
	}
}
