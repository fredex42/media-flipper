package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
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
func ParseSettings(rawString string) (models.TranscodeTypeSettings, error) {
	var avSettings models.JobSettings
	marshalErr := json.Unmarshal([]byte(rawString), &avSettings)

	if marshalErr == nil && avSettings.IsValid() {
		return avSettings, nil
	}

	var imgSettings models.TranscodeImageSettings
	imgMarshalErr := json.Unmarshal([]byte(rawString), &imgSettings)
	if imgMarshalErr == nil && imgSettings.IsValid() {
		return imgSettings, nil
	}

	return nil, errors.New(fmt.Sprintf("could not translate settings: %s and %s", marshalErr, imgMarshalErr))
}

/**
goroutine to monitor the output from the encoding app
*/
func monitorOutput(stdOutChan chan string, stdErrChan chan string, closeChan chan bool, jobContainerId uuid.UUID, jobStepId uuid.UUID) {
	webAppUri := os.Getenv("WEBAPP_BASE") + "/api/transcode/newprogress"

	for {
		select {
		case line := <-stdOutChan:
			log.Print(line)
		case line := <-stdErrChan:

			if strings.HasPrefix(line, "frame=") {
				parsedProgress, parseErr := models.ParseTranscodeProgress(line)
				if parseErr != nil {
					log.Printf("WARNING: Could not parse output: %s. Offending data was '%s'", parseErr, line)
				} else {
					parsedProgress.JobContainerId = jobContainerId
					parsedProgress.JobStepId = jobStepId
					sendErr := SendToWebapp(webAppUri, parsedProgress, 0, 2)
					if sendErr != nil {
						log.Printf("WARNING: Could not update progress in webabb: %s", sendErr)
					}
				}
			} else {
				log.Print(line)
			}
		case <-closeChan:
			log.Print("monitorOutput completed")
			return
		}
	}
}

func RunTranscode(fileName string, maybeOutPath string, settings models.TranscodeTypeSettings, jobContainerId uuid.UUID, jobStepId uuid.UUID) results.TranscodeResult {
	outFileName := GetOutputFileTransc(maybeOutPath, fileName, settings.GetLikelyExtension())

	log.Printf("INFO: RunTranscode output file is %s", outFileName)
	commandArgs := []string{"-i", fileName}
	commandArgs = append(commandArgs, settings.MarshalToArray()...)
	commandArgs = append(commandArgs, "-y", outFileName)

	startTime := time.Now()

	cmd := exec.Command("/usr/bin/ffmpeg", commandArgs...)

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

	go monitorOutput(stdOutChan, stdErrChan, closeChan, jobContainerId, jobStepId)

	waitErr := cmd.Wait()

	closeChan <- true

	endTime := time.Now()
	duration := endTime.UnixNano() - startTime.UnixNano()
	if waitErr != nil {
		log.Printf("Could not execute command: %s", waitErr)
		return results.TranscodeResult{
			OutFile:      "",
			TimeTaken:    float64(duration) / 1e9,
			ErrorMessage: fmt.Sprintf("Could not execute command: %s", waitErr),
		}
	}

	_, statErr := os.Stat(outFileName)

	if statErr != nil {
		log.Printf("Transcode completed but could not find output file: %s", statErr)
		return results.TranscodeResult{
			OutFile:      "",
			TimeTaken:    float64(duration) / 1e9,
			ErrorMessage: fmt.Sprintf("Transcode completed but could not find output file: %s", statErr),
		}
	}

	modErr := os.Chmod(outFileName, 0777)
	if modErr != nil {
		log.Printf("WARNING: Could not change permissions on output file: %s", modErr)
	}
	return results.TranscodeResult{
		OutFile:      outFileName,
		TimeTaken:    float64(duration) / 1e9,
		ErrorMessage: "",
	}
}
