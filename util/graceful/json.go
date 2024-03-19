package graceful

import (
	"encoding/json"
	"net"
	"os"
)

func WriteJSONFile(dst *net.UnixConn, msg any, handle *os.File) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = WriteWithFile(dst, data, handle)
	return err
}

func WriteJSON(dst *net.UnixConn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, _, err = dst.WriteMsgUnix(data, nil, nil)
	return err
}

func ReadJSONFile(src *net.UnixConn, msg any) (*os.File, error) {
	buf := make([]byte, 32*1024)

	n, handle, err := ReadWithFile(src, buf)
	if err != nil {
		return nil, err
	}

	return handle, json.Unmarshal(buf[:n], msg)
}

func ReadJSON(src *net.UnixConn, msg any) error {
	buf := make([]byte, 32*1024)
	tiny := make([]byte, 512)

	n, _, _, _, err := src.ReadMsgUnix(buf, tiny)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf[:n], msg)
}
