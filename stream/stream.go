package stream

import (
	"fmt"
	"regexp"
)

const (
	maxQueueNum = 1024
)

type Stream struct {
	FrameChan chan *Packet
	HasVideo  bool
	HasAudio  bool
	MediaInfo MediaInfo
}

type FrameSlice struct {
	Data    []byte
	RtpData []byte
}

type Packet struct {
	IsVideo      bool
	IsMetadata   bool
	TimeStamp    uint32 // ms
	RtpTimeStamp uint32
	Data         []FrameSlice
	DataLen      int
}

func NewStream(hasVideo, hasAudio bool, mediaInfo MediaInfo) *Stream {
	return &Stream{HasAudio: hasAudio, HasVideo: hasVideo, MediaInfo: mediaInfo, FrameChan: make(chan *Packet, maxQueueNum)}
}

//Send 将数据发送到web前端
func (s *Stream) Send(p *Packet) (ok bool) {
	ok = false
	defer func() {
		if err := recover(); err != nil {
			ok = false
		}
	}()
	s.FrameChan <- p
	return true
}

func (s *Stream) Close() {
	close(s.FrameChan)
}

func StreamID(url string) (string, error) {
	str := url
	r, err := regexp.Compile(`.*/(\w+)`)
	if err != nil {
		return "", err
	}
	ss := r.FindStringSubmatch(str)
	if len(ss) >= 2 {
		return ss[1], nil
	}
	return "", fmt.Errorf("url format err")
}
