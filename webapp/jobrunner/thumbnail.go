package jobrunner

import (
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	models2 "github.com/guardian/mediaflipper/common/models"
	v1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	v13 "k8s.io/client-go/kubernetes/typed/core/v1"
	"log"
)

func CreateThumbnailJob(jobDesc models2.JobStepThumbnail, maybeOutPath string, jobClient v1.JobInterface, svcClient v13.ServiceInterface) error {
	if jobDesc.MediaFile == "" {
		log.Printf("Can't perform thumbnail with no media file")
		return errors.New("can't perform thumbnail with no media file")
	}

	var thumbFrameSeconds float64

	var jsonTranscodeSettings []byte
	if jobDesc.TranscodeSettings != nil {
		var marshalErr error
		jsonTranscodeSettings, marshalErr = jobDesc.TranscodeSettings.InternalMarshalJSON()
		if marshalErr != nil {
			log.Printf("Could not convert settings into json: %s", marshalErr)
			log.Printf("Offending data was %s", spew.Sdump(jobDesc.TranscodeSettings))
			return marshalErr
		}
	}
	vars := map[string]string{
		"WRAPPER_MODE":       "thumbnail",
		"JOB_CONTAINER_ID":   jobDesc.JobContainerId.String(),
		"JOB_STEP_ID":        jobDesc.JobStepId.String(),
		"FILE_NAME":          jobDesc.MediaFile,
		"TRANSCODE_SETTINGS": string(jsonTranscodeSettings),
		"THUMBNAIL_FRAME":    fmt.Sprintf("%f", thumbFrameSeconds),
		"MAX_RETRIES":        "10",
		"MEDIA_TYPE":         string(jobDesc.ItemType),
		"OUTPUT_PATH":        maybeOutPath,
	}

	//jobName := fmt.Sprintf("mediaflipper-thumbnail-%s", path.Base(jobDesc.MediaFile))
	return CreateGenericJob(jobDesc.JobStepId, "flip-thumb", vars, true, jobDesc.KubernetesTemplateFile, jobClient, svcClient)
}
