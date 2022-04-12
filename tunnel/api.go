package tunnel

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/andrewstucki/light/tunnel/proto"
)

// apiRequestFromProto
func apiRequestFromProto(ctx context.Context, req *proto.APIRequest) (*http.Request, error) {
	body := bytes.NewReader(req.Body)
	httpReq, err := http.NewRequest(req.RequestMethod, req.RequestUrl, body)
	if err != nil {
		return nil, err
	}
	httpReq.Header = pairsToHeaders(req.Headers)
	parameters := httpReq.URL.Query()
	for _, parameter := range req.Parameters {
		parameters.Add(parameter.Name, parameter.Value)
	}

	return httpReq.WithContext(ctx), nil
}

// httpRequestToProto
func httpRequestToProto(req *http.Request) (*proto.APIRequest, error) {
	defer req.Body.Close()
	// add 1 byte to easily see if we exceeded the limit
	reader := io.LimitReader(req.Body, maxRequestSize+1)

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if len(data) > maxRequestSize {
		return nil, io.EOF
	}
	return &proto.APIRequest{
		RequestMethod: req.Method,
		RequestUrl:    req.URL.Path,
		Headers:       headersToPairs(req.Header),
		Parameters:    valuesToPairs(req.URL.Query()),
		Body:          data,
	}, nil
}

type apiResponse struct {
	status  int
	headers http.Header
	body    bytes.Buffer
}

var _ http.ResponseWriter = &apiResponse{}

// newAPIResponse
func newAPIResponse() *apiResponse {
	return &apiResponse{
		headers: make(http.Header),
	}
}

// toProto
func (a *apiResponse) toProto() *proto.APIResponse {
	if a.body.Len() > maxBodySize {
		return &proto.APIResponse{
			Status: int64(http.StatusRequestEntityTooLarge),
			Body:   []byte("response too large"),
		}
	}
	return &proto.APIResponse{
		Status:  int64(a.status),
		Body:    a.body.Bytes(),
		Headers: headersToPairs(a.headers),
	}
}

// convert
func convert(protoResponse *proto.APIResponse, response http.ResponseWriter) error {
	headers := response.Header()
	for _, pair := range protoResponse.Headers {
		headers.Add(pair.Name, pair.Value)
	}
	response.WriteHeader(int(protoResponse.Status))
	_, err := response.Write(protoResponse.Body)
	return err
}

func (a *apiResponse) Header() http.Header {
	return a.headers
}

func (a *apiResponse) Write(data []byte) (int, error) {
	return a.body.Write(data)
}

func (a *apiResponse) WriteHeader(statusCode int) {
	a.status = statusCode
}

func headersToPairs(headers http.Header) []*proto.Pair {
	pairs := []*proto.Pair{}
	for name, header := range headers {
		for _, value := range header {
			pairs = append(pairs, &proto.Pair{
				Name:  name,
				Value: value,
			})
		}
	}
	return pairs
}

func valuesToPairs(values url.Values) []*proto.Pair {
	pairs := []*proto.Pair{}
	for name, parameters := range values {
		for _, value := range parameters {
			pairs = append(pairs, &proto.Pair{
				Name:  name,
				Value: value,
			})
		}
	}
	return pairs
}

func pairsToHeaders(pairs []*proto.Pair) http.Header {
	headers := make(http.Header)
	for _, pair := range pairs {
		headers.Add(pair.Name, pair.Value)
	}
	return headers
}
