package main

import (
	"flag"
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

/**
we expect the following environment variables to be set:
WRAPPER_MODE={analyse|thumbnail|transcode}
JOB_STEP_ID={uuid-string}
JOB_CONTAINER_ID={uuid-string}
WEBAPP_BASE={url-string}  [url to contact main webapp]
MAX_RETRIES={count}
THUMBNAIL_FRAME={int} [thumbnail only]
TRANSCODE_SETTINGS={jsonstring} [transcode only]
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
		result, err := RunAnalysis(filename)

		if err != nil {
			log.Fatal("Could not run analysis: ", err)
		}

		log.Print("Got analysis result: ", result)
		sendUrl := os.Getenv("WEBAPP_BASE") + "/api/analysis/result?forJob=" + os.Getenv("JOB_CONTAINER_ID") + "&stepId=" + os.Getenv("JOB_STEP_ID")
		sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
		if sendErr != nil {
			log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
		}
		break
	case "thumbnail":
		var thumbFrame int
		if os.Getenv("THUMBNAIL_FRAME") != "" {
			thumbFrame64, _ := strconv.ParseInt(os.Getenv("THUMBNAIL_FRAME"), 10, 32)
			thumbFrame = int(thumbFrame64)
		} else {
			thumbFrame = 30
		}

		result := RunThumbnail(filename, thumbFrame)
		log.Print("Got thumbnail result: ", result)

		sendUrl := os.Getenv("WEBAPP_BASE") + "/api/thumbnail/result?forJob=" + os.Getenv("JOB_CONTAINER_ID") + "&stepId=" + os.Getenv("JOB_STEP_ID")
		sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
		if sendErr != nil {
			log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
		}
		break
	case "transcode":
		log.Printf("Raw transcode settings: %s", os.Getenv("TRANSCODE_SETTINGS"))
		transcodeSettings, settingsErr := ParseSettings(os.Getenv("TRANSCODE_SETTINGS"))
		if settingsErr != nil {
			log.Fatalf("Could not parse settings from TRANSCODE_SETTINGS var: %s", settingsErr)
		}
		result := RunTranscode(filename, transcodeSettings)
		log.Print("Got transcode result: ", result)

		sendUrl := os.Getenv("WEBAPP_BASE") + "/api/transcode/result?forJob=" + os.Getenv("JOB_CONTAINER_ID") + "&stepId=" + os.Getenv("JOB_STEP_ID")
		sendErr := SendToWebapp(sendUrl, result, 0, maxTries)
		if sendErr != nil {
			log.Fatalf("Could not send results to %s: %s", sendUrl, sendErr)
		}
		break
	default:
		log.Fatalf("WRAPPER_MODE '%s' is not recognised", os.Getenv("WRAPPER_MODE"))
		break
	}

}
