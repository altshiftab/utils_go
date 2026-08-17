package http_request

import "github.com/altshiftab/utils_go/pkg/cloud/gcp/types/log_entry/duration"

type Request struct {
	RequestMethod                  string             `json:"requestMethod,omitempty"`
	RequestUrl                     string             `json:"requestUrl,omitempty"`
	RequestSize                    int                `json:"requestSize,omitempty"`
	Status                         int                `json:"status,omitempty"`
	ResponseSize                   int                `json:"responseSize,omitempty"`
	UserAgent                      string             `json:"userAgent,omitempty"`
	RemoteIp                       string             `json:"remoteIp,omitempty"`
	ServerIp                       string             `json:"serverIp,omitempty"`
	Referer                        string             `json:"referer,omitempty"`
	Latency                        *duration.Duration `json:"latency,omitempty"`
	CacheLookup                    *bool              `json:"cacheLookup,omitempty"`
	CacheHit                       *bool              `json:"cacheHit,omitempty"`
	CacheValidatedWithOriginServer *bool              `json:"cacheValidatedWithOriginServer,omitempty"`
	CacheFillBytes                 int                `json:"cacheFillBytes,omitempty"`
	Protocol                       string             `json:"protocol,omitempty"`
}
