package transcode

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-redis/redis/v7"
	"github.com/guardian/mediaflipper/common/helpers"
	models2 "github.com/guardian/mediaflipper/common/models"
	"github.com/guardian/mediaflipper/common/results"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
)

type ReceiveData struct {
	redisClient *redis.Client
}

func (h ReceiveData) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if !helpers.AssertHttpMethod(r, w, "POST") {
		return //response should already be sent here
	}

	_, jobContainerId, jobStepId, paramsErr := helpers.GetReceiverJobIds(r.RequestURI)
	if paramsErr != nil {
		//paramsErr is NOT a golang error object, but a premade GenericErrorResponse
		helpers.WriteJsonContent(paramsErr, w, 400)
		return
	}

	var incoming results.TranscodeResult
	rawContent, _ := ioutil.ReadAll(r.Body)
	parseErr := json.Unmarshal(rawContent, &incoming)

	if parseErr != nil {
		log.Printf("Could not understand content body: %s", parseErr)
		log.Printf("Offending data was: %s", string(rawContent))
		helpers.WriteJsonContent(helpers.GenericErrorResponse{"bad_request", parseErr.Error()}, w, 400)
		return
	}

	var fileEntry *models2.FileEntry = nil
	if incoming.OutFile != "" {
		f, fileEntryErr := models2.NewFileEntry(incoming.OutFile, *jobContainerId, models2.TYPE_TRANSCODE)
		if fileEntryErr != nil {
			log.Printf("Could not get information for incoming thumbnail %s: %s", spew.Sprint(incoming), fileEntryErr)
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"error", "could not get file info"}, w, 500)
			return
		}
		fileEntry = &f
		storErr := fileEntry.Store(h.redisClient)
		if storErr != nil {
			log.Printf("Could not store file entry: %s", storErr)
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"db_error", storErr.Error()}, w, 500)
			return
		}
	}

	completionChan := make(chan bool)

	whenQueueReady := func(waitErr error) {
		if waitErr != nil {
			log.Printf("queue wait failed: %s", waitErr)
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"error", "queue wait failed, see the logs"}, w, 500)
			completionChan <- false
			return
		}

		jobContainerInfo, containerGetErr := models2.JobContainerForId(*jobContainerId, h.redisClient)
		if containerGetErr != nil {
			log.Printf("Could not retrieve job container for %s: %s", jobContainerId.String(), containerGetErr)
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"db_error", "Invalid job id"}, w, 400)
			completionChan <- false
			return
		}

		jobStepCopyPtr := jobContainerInfo.FindStepById(*jobStepId)
		if jobStepCopyPtr == nil {
			log.Printf("Job container %s does not have any step with the id %s", jobContainerId.String(), jobStepId.String())
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"not_found", "no jobstep with that id in the given job"}, w, 404)
			completionChan <- false
			return
		}

		tcStep, isTc := (*jobStepCopyPtr).(*models2.JobStepTranscode)
		if !isTc {
			log.Printf("Job step was not transcode type, got %s", reflect.TypeOf(jobStepCopyPtr))
			helpers.WriteJsonContent(helpers.GenericErrorResponse{
				Status: "bad_request",
				Detail: "job step was not transcode",
			}, w, 400)
			completionChan <- false
			return
		}

		if fileEntry != nil {
			tcStep.ResultId = &(fileEntry.Id)
			jobContainerInfo.TranscodedMediaId = &(fileEntry.Id)
		}

		var updatedStep models2.JobStep
		if incoming.ErrorMessage != "" {
			updatedStep = tcStep.WithNewStatus(models2.JOB_FAILED, &incoming.ErrorMessage)
		} else {
			updatedStep = tcStep.WithNewStatus(models2.JOB_COMPLETED, nil)
		}

		updateErr := jobContainerInfo.UpdateStepById(updatedStep.StepId(), updatedStep)
		if updateErr != nil {
			log.Printf("Could not update job container with new step: %s", updateErr)
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"error", "could not update job container"}, w, 500)
			completionChan <- false
			return
		}

		log.Printf("Storing updated container...")

		storErr := jobContainerInfo.Store(h.redisClient)
		if storErr != nil {
			log.Printf("Could not store job container: %s", storErr)
			helpers.WriteJsonContent(helpers.GenericErrorResponse{"db_error", "could not store updated content"}, w, 200)
			completionChan <- false
			return
		}

		helpers.WriteJsonContent(helpers.GenericErrorResponse{"ok", "result saved"}, w, 200)
		completionChan <- true
	}

	models2.WhenQueueAvailable(h.redisClient, models2.RUNNING_QUEUE, whenQueueReady, true)
	<-completionChan
}
