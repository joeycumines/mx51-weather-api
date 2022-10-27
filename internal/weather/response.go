package weather

import (
	"encoding/json"
	"fmt"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strconv"
)

func writeError(w http.ResponseWriter, statusCode int, err error) error {
	if err == nil {
		panic(`writeError: non-nil error required`)
	}
	var sts *statuspb.Status
	{
		v, _ := status.FromError(err)
		sts = v.Proto()
	}
	// note: sts should always be non-nil
	return writeProtoJSON(w, statusCode, sts)
}

func writeProtoJSON(w http.ResponseWriter, statusCode int, msg proto.Message) error {
	b, err := protojson.Marshal(msg)
	if err != nil {
		return err
	}
	return writeJSON(w, statusCode, json.RawMessage(b))
}

func writeJSON(w http.ResponseWriter, statusCode int, JSON any) error {
	b, err := json.Marshal(JSON)
	if err != nil {
		return err
	}
	return writeBytes(w, statusCode, `application/json`, b)
}

func writeBytes(w http.ResponseWriter, statusCode int, contentType string, body []byte) (err error) {
	w.Header().Set(`Content-Type`, contentType)
	w.Header().Set(`Content-Length`, strconv.Itoa(len(body)))
	w.WriteHeader(statusCode)
	var n int
	n, err = w.Write(body)
	if err == nil && n != len(body) {
		err = fmt.Errorf(`short write: expected %d actual %d`, len(body), n)
	}
	return
}
