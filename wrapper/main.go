package main

import (
	"flag"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/guardian/mediaflipper/common/models"
	"github.com/guardian/mediaflipper/common/results"
	"log"
	"os"
	"strconv"
)

func GetMaxRetries() int {
	stringVal := os.Getenv("MAX_RETRIES")
	if stringVal != "" {
		value, err := strconv.ParseInt(stringVal, 10, 16)
		if err != nil {
			log.Fatalf("Invalid value for MAX_RETRIES (not an integer): %s", err)
		}
		return int(value)
	} else {
		return 10 //default value
	}
}

func EnsureOutputPath(sendUrl string, maxTries int) {
	maybeOutPath := os.Getenv("OUTPUT_PATH")
	if maybeOutPath != "" {
		log.Printf("INFO: ensuring output directory %s exists", maybeOutPath)
		statInfo, statErr := os.Stat(maybeOutPath)
		if statErr == nil {
			if !statInfo.IsDir() {
				result := results.TranscodeResult{
					OutFile:      "",
					TimeTaken:    0,
					ErrorMessage: fmt.Sprintf("output path %s existed but was not a directory!", maybeOutPath),
				}
				sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
				if sendErr != nil {
					log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
				}
				os.Exit(1)
			}
		} else {
			if os.IsNotExist(statErr) {
				log.Printf("INFO: creating directory %s", maybeOutPath)
				makeErr := os.MkdirAll(maybeOutPath, 0777)
				if makeErr != nil {
					result := results.TranscodeResult{
						OutFile:      "",
						TimeTaken:    0,
						ErrorMessage: fmt.Sprintf("could not create output path %s: %s", maybeOutPath, makeErr),
					}
					sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
					if sendErr != nil {
						log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
					}
					os.Exit(1)
				}
			}
		}
	}
}

/**
we expect the following environment variables to be set:
WRAPPER_MODE={analyse|thumbnail|transcode}
JOB_STEP_ID={uuid-string}
JOB_CONTAINER_ID={uuid-string}
WEBAPP_BASE={url-string}  [url to contact main webapp]
MAX_RETRIES={count}
THUMBNAIL_FRAME={int} [thumbnail only]
TRANSCODE_SETTINGS={jsonstring} [transcode only]
MEDIA_TYPE={video|audio|image|other}
OUTPUT_PATH={optional path to output. defaults to same location as incoming media}
*/
func main() {
	testFilePtr := flag.String("filename", "", "testing option, run on this file")
	flag.Parse()

	maxTries := GetMaxRetries()
	log.Printf("Max retriues set to %d", maxTries)
	var filename string
	if os.Getenv("FILE_NAME") != "" {
		filename = os.Getenv("FILE_NAME")
	} else {
		filename = *testFilePtr
	}

	switch os.Getenv("WRAPPER_MODE") {
	case "analyse":
		sendUrl := os.Getenv("WEBAPP_BASE") + "/api/analysis/result?forJob=" + os.Getenv("JOB_CONTAINER_ID") + "&stepId=" + os.Getenv("JOB_STEP_ID")
		EnsureOutputPath(sendUrl, maxTries)

		result, err := RunAnalysis(filename)

		if err != nil {
			log.Fatal("Could not run analysis: ", err)
		}

		log.Print("Got analysis result: ", result)
		sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
		if sendErr != nil {
			log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
		}
		break
	case "thumbnail":
		sendUrl := os.Getenv("WEBAPP_BASE") + "/api/thumbnail/result?forJob=" + os.Getenv("JOB_CONTAINER_ID") + "&stepId=" + os.Getenv("JOB_STEP_ID")
		EnsureOutputPath(sendUrl, maxTries)

		var thumbFrame int
		if os.Getenv("THUMBNAIL_FRAME") != "" {
			thumbFrame64, _ := strconv.ParseInt(os.Getenv("THUMBNAIL_FRAME"), 10, 32)
			thumbFrame = int(thumbFrame64)
		} else {
			thumbFrame = 30
		}

		var result *ThumbnailResult
		rawSettings := os.Getenv("TRANSCODE_SETTINGS")
		if rawSettings != "" {
			transcodeSettings, settingsErr := ParseSettings(os.Getenv("TRANSCODE_SETTINGS"))
			if settingsErr != nil {
				log.Fatalf("Could not parse settings from TRANSCODE_SETTINGS var: %s", settingsErr)
			}
			if _, isImage := transcodeSettings.(models.TranscodeImageSettings); isImage {
				log.Printf("Performing image thumbnail with provided settings...")
				result = RunImageThumbnail(filename, os.Getenv("OUTPUT_PATH"), transcodeSettings)
			}
			if _, isAV := transcodeSettings.(models.JobSettings); isAV {
				log.Printf("Performing video thumbnail with provided settings...")
				result = RunVideoThumbnail(filename, os.Getenv("OUTPUT_PATH"), thumbFrame)
			}
		} else {
			log.Printf("Performing video thumbnail by default with no provided settings...")
			result = RunVideoThumbnail(filename, os.Getenv("OUTPUT_PATH"), thumbFrame)
		}

		log.Print("Got thumbnail result: ", result)

		sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
		if sendErr != nil {
			log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
		}
		break
	case "transcode":
		sendUrl := os.Getenv("WEBAPP_BASE") + "/api/transcode/result?forJob=" + os.Getenv("JOB_CONTAINER_ID") + "&stepId=" + os.Getenv("JOB_STEP_ID")
		EnsureOutputPath(sendUrl, maxTries)

		log.Printf("Raw transcode settings: %s", os.Getenv("TRANSCODE_SETTINGS"))
		transcodeSettings, settingsErr := ParseSettings(os.Getenv("TRANSCODE_SETTINGS"))
		if settingsErr != nil {
			log.Fatalf("Could not parse settings from TRANSCODE_SETTINGS var: %s", settingsErr)
		}
		jobId, jobIdErr := uuid.Parse(os.Getenv("JOB_CONTAINER_ID"))
		if jobIdErr != nil {
			log.Fatal("Could not parse JOB_CONTAINER_ID as a uuid: ", jobIdErr)
		}
		stepId, stepIdErr := uuid.Parse(os.Getenv("JOB_STEP_ID"))
		if stepIdErr != nil {
			log.Fatal("Could not parse JOB_STEP_ID as a uuid: ", stepIdErr)
		}

		var result results.TranscodeResult
		avSettings, isAv := transcodeSettings.(models.JobSettings)
		if isAv {
			result = RunTranscode(filename, os.Getenv("OUTPUT_PATH"), avSettings, jobId, stepId)
			log.Print("Got transcode result: ", result)
		} else {
			log.Printf("Could not recognise settings type for %s", spew.Sdump(transcodeSettings))
			result = results.TranscodeResult{
				OutFile:      "",
				TimeTaken:    0,
				ErrorMessage: "could not recognise settings as valid for a transcode operation. Maybe you meant thumbnail?",
			}
		}

		sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
		if sendErr != nil {
			log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
		}
	default:
		log.Fatalf("WRAPPER_MODE '%s' is not recognised", os.Getenv("WRAPPER_MODE"))
	}

}
