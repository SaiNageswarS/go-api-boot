package server

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
)

func interceptGrpcRequest(req *http.Request) (*http.Request, bool) {
	// Hack, this should be a shallow copy, but let's see if this works
	req.ProtoMajor = 2
	req.ProtoMinor = 0

	contentType := req.Header.Get("content-type")
	incomingContentType := grpcWebContentType
	isTextFormat := strings.HasPrefix(contentType, grpcWebTextContentType)
	if isTextFormat {
		// body is base64-encoded: decode it; Wrap it in readerCloser so Body is still closed
		decoder := base64.NewDecoder(base64.StdEncoding, req.Body)
		req.Body = &readerCloser{reader: decoder, closer: req.Body}
		incomingContentType = grpcWebTextContentType
	}
	req.Header.Set("content-type", strings.Replace(contentType, incomingContentType, grpcContentType, 1))

	// Remove content-length header since it represents http1.1 payload size, not the sum of the h2
	// DATA frame payload lengths. https://http2.github.io/http2-spec/#malformed This effectively
	// switches to chunked encoding which is the default for h2
	req.Header.Del("content-length")

	return req, isTextFormat
}

// readerCloser combines an io.Reader and an io.Closer into an io.ReadCloser.
type readerCloser struct {
	reader io.Reader
	closer io.Closer
}

func (r *readerCloser) Read(dest []byte) (int, error) {
	return r.reader.Read(dest)
}

func (r *readerCloser) Close() error {
	return r.closer.Close()
}
