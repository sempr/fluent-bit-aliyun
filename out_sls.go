package main

import (
	"C"
	"fmt"
	"os"
	"reflect"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	sls "github.com/galaxydi/go-loghub"
	"github.com/gogo/protobuf/proto"
	"github.com/ugorji/go/codec"
)

var project *sls.LogProject
var logstore *sls.LogStore
var accessKey, accessKeySecret, projectName, endpoint, logstoreName string

//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	accessKey = output.FLBPluginConfigKey(ctx, "accessKey")
	accessKeySecret = output.FLBPluginConfigKey(ctx, "accessKeySecret")
	projectName = output.FLBPluginConfigKey(ctx, "projectName")
	endpoint = output.FLBPluginConfigKey(ctx, "endpoint")
	logstoreName = output.FLBPluginConfigKey(ctx, "logstoreName")
	return FLBPluginRegister(ctx)
}

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	if accessKey == "":
		accessKey = os.Getenv("ALIYUN_ACCESS_KEY")
	if accessKeySecret == "":
		accessKeySecret = os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	if projectName == ""
		projectName = os.Getenv("ALIYUN_SLS_PROJECT")
	if endpoint == "":
		endpoint = os.Getenv("ALIYUN_SLS_ENDPOINT")
	if logstoreName == "":
	logstoreName = os.Getenv("ALIYUN_SLS_LOGSTORE")

	project = &sls.LogProject{
		Name:            projectName,
		Endpoint:        endpoint,
		AccessKeyID:     accessKey,
		AccessKeySecret: accessKeySecret,
	}

	var err error
	logstore, err = project.GetLogStore(logstoreName)
	if err != nil {
		fmt.Printf("Unable to get logstore %v: %v", logstoreName, err)
		return output.FLB_ERROR
	}
	return output.FLBPluginRegister(ctx, "sls", "Aliyun SLS output")
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	var h codec.Handle = new(codec.MsgpackHandle)
	var b []byte
	var m interface{}
	var err error

	if logstore == nil {
		fmt.Printf("logstore is nil")
		return output.FLB_ERROR
	}

	b = C.GoBytes(data, length)
	dec := codec.NewDecoderBytes(b, h)

	// Iterate the original MessagePack array
	logs := []*sls.Log{}
	for {
		// Decode the entry
		err = dec.Decode(&m)
		if err != nil {
			break
		}

		// Get a slice and their two entries: timestamp and map
		slice := reflect.ValueOf(m)
		timestampData := slice.Index(0)
		data := slice.Index(1)
		timestamp, ok := timestampData.Interface().(uint64)
		if !ok {
			fmt.Printf("Unable to convert timestamp: %+v", timestampData)
			return output.FLB_ERROR
		}

		// Convert slice data to a real map and iterate
		mapData := data.Interface().(map[interface{}]interface{})
		flattenData, err := Flatten(mapData, "", UnderscoreStyle)
		if err != nil {
			break
		}
		content := []*sls.LogContent{}
		for k, v := range flattenData {
			value := ""
			switch t := v.(type) {
			case string:
				value = t
			case []byte:
				value = string(t)
			default:
				value = fmt.Sprintf("%v", v)
			}
			content = append(content, &sls.LogContent{
				Key:   proto.String(k),
				Value: proto.String(value),
			})
		}
		log := &sls.Log{
			Time:     proto.Uint32(uint32(timestamp)),
			Contents: content,
		}
		logs = append(logs, log)
	}
	loggroup := &sls.LogGroup{
		Topic:  proto.String(C.GoString(tag)),
		Source: proto.String(""),
		Logs:   logs,
	}
	err = logstore.PutLogs(loggroup)
	if err != nil {
		return output.FLB_ERROR
	}

	// Return options:
	//
	// output.FLB_OK    = data have been processed.
	// output.FLB_ERROR = unrecoverable error, do not try this again.
	// output.FLB_RETRY = retry to flush later.
	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	return 0
}

func main() {
}
