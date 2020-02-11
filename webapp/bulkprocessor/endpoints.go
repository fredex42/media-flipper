package bulkprocessor

import (
	"github.com/go-redis/redis/v7"
	"net/http"
)

type BulkEndpoints struct {
	GetHandler      GetHandler
	UploadHandler   BulkListUploader
	ListHandler     ListHandler
	ContentsHandler ContentsHandler
}

func NewBulkEndpoints(redisClient *redis.Client) BulkEndpoints {
	return BulkEndpoints{
		GetHandler:      GetHandler{redisClient: redisClient},
		UploadHandler:   BulkListUploader{redisClient: redisClient},
		ListHandler:     ListHandler{redisClient: redisClient},
		ContentsHandler: ContentsHandler{redisClient: redisClient},
	}
}

func (e BulkEndpoints) WireUp(baseUrl string) {
	http.Handle(baseUrl+"/get", e.GetHandler)
	http.Handle(baseUrl+"/upload", e.UploadHandler)
	http.Handle(baseUrl+"/list", e.ListHandler)
	http.Handle(baseUrl+"/content", e.ContentsHandler)
}
