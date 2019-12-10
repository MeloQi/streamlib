package rtp

import (
	"bytes"
	"fmt"
	"github.com/MeloQi/streamlib/utils"
	"math"
)

const (
	MAX_RTP_LEN    = 1456
	RTP_HEADER_LEN = 12
)

type RtpTransfer struct {
	CurFrame  *FrameInfo
	outRTPbuf []byte
	cseq      uint16
	//for pts
	ctime                        float64 //play time sec
	preRTPTimestamp              uint64
	rtpTimeIntervalMedian        uint64
	rtpTimeIntervalStatistics    map[uint64]int
	rtpTimeIntervalStatisticsCnt int
}

func NewRRtpTransfer() *RtpTransfer {
	return &RtpTransfer{
		NewFrameInfo(),
		make([]byte, MAX_RTP_LEN),
		0,
		0.0,
		0,
		0,
		make(map[uint64]int),
		0,
	}
}

func (rt *RtpTransfer) CalPts(Timestamp uint64) int64 {
	if rt.preRTPTimestamp == 0 { //start
	} else if uint64(Timestamp) < rt.preRTPTimestamp { //reset
		rt.ctime = rt.ctime + float64(rt.rtpTimeIntervalMedian)/90000
	} else if uint64(Timestamp) > rt.preRTPTimestamp {
		interval := uint64(Timestamp) - rt.preRTPTimestamp
		if cnt, ok := rt.rtpTimeIntervalStatistics[interval]; ok {
			rt.rtpTimeIntervalStatistics[interval] = cnt + 1
		} else {
			rt.rtpTimeIntervalStatistics[interval] = 1
		}
		rt.rtpTimeIntervalStatisticsCnt++
		if rt.rtpTimeIntervalStatisticsCnt > 20 {
			rt.rtpTimeIntervalStatisticsCnt = 0
			for k, v := range rt.rtpTimeIntervalStatistics {
				if _, ok := rt.rtpTimeIntervalStatistics[rt.rtpTimeIntervalMedian]; !ok {
					rt.rtpTimeIntervalMedian = k
				}
				if v > rt.rtpTimeIntervalStatistics[rt.rtpTimeIntervalMedian] {
					rt.rtpTimeIntervalMedian = k
				}
			}
			for k, _ := range rt.rtpTimeIntervalStatistics {
				delete(rt.rtpTimeIntervalStatistics, k)
			}
		}

		rt.ctime = rt.ctime + float64(interval)/90000
	}
	rt.preRTPTimestamp = uint64(Timestamp)

	return int64(uint64((27000000*rt.ctime)/300) % uint64(math.Pow(2, 33)))
}

type RTPHeaderInfo struct {
	V                     uint8 //2bit
	IsPadding             bool  //1bit
	PaddLen               uint8
	Isextension           bool //1bit
	ExtensionDefByProfile uint16
	ExtensionLen          uint16
	CC                    uint8  // 4bit
	IsMark                bool   //1bit
	PayloadType           uint8  //7bit
	Cseq                  uint16 //16bit
	Timestamp             uint32 //32bit
	SSRC                  uint32 //32bit
	Payload               []byte
}

type FrameInfo struct {
	NaluType      int
	Frame         []byte
	DataLen       int
	SubFrameInfos []SubFrameInfo
	SSRC          uint32 //rtp
	Timestamp     uint32 //rtp
}

type SubFrameInfo struct {
	NaluType int
	Data     []byte
	RtpData  []byte
}

type RTPPack struct {
	Type   RTPType
	Buffer *bytes.Buffer
}

type RTPType int

const (
	RTP_TYPE_AUDIO RTPType = iota
	RTP_TYPE_VIDEO
	RTP_TYPE_AUDIOCONTROL
	RTP_TYPE_VIDEOCONTROL
)

func (rt RTPType) String() string {
	switch rt {
	case RTP_TYPE_AUDIO:
		return "audio"
	case RTP_TYPE_VIDEO:
		return "video"
	case RTP_TYPE_AUDIOCONTROL:
		return "audio control"
	case RTP_TYPE_VIDEOCONTROL:
		return "video control"
	}
	return "unknow"
}

const FrameMaxLen int = 1024 * 1024

func NewFrameInfo() *FrameInfo {
	return &FrameInfo{Frame: make([]byte, FrameMaxLen)}
}

func ResetFrameInfo(frame *FrameInfo, rtpHeaderInfo *RTPHeaderInfo) {
	frame.SSRC = rtpHeaderInfo.SSRC
	frame.Timestamp = rtpHeaderInfo.Timestamp
	frame.DataLen = 0
	frame.SubFrameInfos = []SubFrameInfo{}
}

func (rtp *RtpTransfer) GetH264FrameSlices(rtpData []byte) (frameSlices *FrameInfo, err error) {
	frameSlices = rtp.CurFrame
	if rtpData == nil || len(rtpData) < 12 {
		frameSlices = nil
		err = fmt.Errorf("rtp format err")
		return
	}

	rtpHeaderInfo := ParseRTPHeader(rtpData)
	if rtpHeaderInfo == nil {
		frameSlices = nil
		err = fmt.Errorf("Parse RTP Header ERR")
		return
	}

	nalu := rtpHeaderInfo.Payload
	naluType := nalu[0] & 0x1f
	switch {
	case naluType >= 1 && naluType <= 23:
		ResetFrameInfo(frameSlices, rtpHeaderInfo)
		frameSlices.NaluType = int(naluType)
		frameSlices.DataLen = len(rtpHeaderInfo.Payload)
		dataHasRTP := rtpData[len(rtpData)-len(rtpHeaderInfo.Payload)-RTP_HEADER_LEN:]
		frameSlices.SubFrameInfos = append(frameSlices.SubFrameInfos, SubFrameInfo{NaluType: int(naluType), Data: rtpHeaderInfo.Payload, RtpData: dataHasRTP})
		return frameSlices, nil

	case naluType == 28: // FU-A
		fuIndicator := nalu[0]
		fuHeader := nalu[1]
		isStart := fuHeader&0x80 != 0
		isEnd := fuHeader&0x40 != 0
		dataHasRTP := rtpData[len(rtpData)-len(rtpHeaderInfo.Payload)-RTP_HEADER_LEN+2:]
		if isStart {
			ResetFrameInfo(frameSlices, rtpHeaderInfo)
			frameSlices.NaluType = int(fuHeader & 0x1f)
			frameSlices.SubFrameInfos = append(frameSlices.SubFrameInfos, SubFrameInfo{NaluType: frameSlices.NaluType, Data: []byte{(fuIndicator & 0xe0) | (fuHeader & 0x1f)}, RtpData: nil})
			frameSlices.DataLen += 1
			frameSlices.SubFrameInfos = append(frameSlices.SubFrameInfos, SubFrameInfo{NaluType: frameSlices.NaluType, Data: nalu[2:], RtpData: dataHasRTP})
			frameSlices.DataLen += len(nalu[2:])
		} else {
			frameSlices.SubFrameInfos = append(frameSlices.SubFrameInfos, SubFrameInfo{NaluType: frameSlices.NaluType, Data: nalu[2:], RtpData: dataHasRTP})
			frameSlices.DataLen += len(nalu[2:])
		}
		if isEnd {
			return frameSlices, nil
		} else {
			return nil, nil
		}

	case naluType == 24: // STAP-A
		/*
			0                   1                   2                   3
			0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
			+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			|                          RTP Header      rtp STAP-A                     |
			+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			|STAP-A NAL HDR |         NALU 1 Size           | NALU 1 HDR    |
			+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			|                         NALU 1 Data                           |
			:                                                               :
			+               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			|               | NALU 2 Size                   | NALU 2 HDR    |
			+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			|                         NALU 2 Data                           |
			:                                                               :
			|                               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			|                               :...OPTIONAL RTP padding        |
			+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		*/
		ResetFrameInfo(frameSlices, rtpHeaderInfo)
		nalu = nalu[1:]
		for len(nalu) >= 2 {
			size := int(nalu[0])<<8 | int(nalu[1])
			if size+2 > len(nalu) {
				//Failed to parse STAP-A
				err = fmt.Errorf("rtp STAP-A: size(%d) is more than payload's length(%d)", size+2, len(nalu))
				break
			}
			nalu = nalu[2:]
			frameSlices.SubFrameInfos = append(frameSlices.SubFrameInfos, SubFrameInfo{int(nalu[0] & 0x1f), nalu[:size], nil})
			frameSlices.DataLen += size
			nalu = nalu[size:]
		}
		return frameSlices, nil

	default:
		frameSlices = nil
		err = fmt.Errorf("rtsp: unsupported H264 naluType=%d", naluType)
		return
	}
}

func (rtp *RtpTransfer) PkgRtpOut(data []byte, rtpType RTPType, hasRTPHeader bool, rtpPayloadType uint8, isMarker bool, timestamp uint32, ssrc uint32, outCallBack func(pack *RTPPack)) {
	if data == nil || len(data) == 0 || outCallBack == nil {
		return
	}

	if hasRTPHeader {
		rtp.FillRtpHeader(data, rtpPayloadType, isMarker, timestamp, ssrc)
		outCallBack(&RTPPack{Type: rtpType, Buffer: bytes.NewBuffer(data)})
		return
	}

	outBuf := rtp.outRTPbuf
	for len(data) > MAX_RTP_LEN-RTP_HEADER_LEN {
		rtp.FillRtpHeader(outBuf[:12], rtpPayloadType, false, timestamp, ssrc)
		copy(outBuf[12:], data[:MAX_RTP_LEN-12])
		data = data[MAX_RTP_LEN-12:]
		outCallBack(&RTPPack{Type: rtpType, Buffer: bytes.NewBuffer(outBuf[:MAX_RTP_LEN])})
	}
	rtp.FillRtpHeader(outBuf[:12], rtpPayloadType, true, timestamp, ssrc)
	copy(outBuf[12:], data)
	outCallBack(&RTPPack{Type: rtpType, Buffer: bytes.NewBuffer(outBuf[:len(data)+12])})
}

func (rtp *RtpTransfer) FillRtpHeader(outHeader []byte, rtpPayloadType uint8, isMarker bool, timestamp uint32, ssrc uint32) {
	rtp.cseq++
	bits := utils.BitsInit(12, outHeader)
	utils.BitsWrite(bits, 2, 2)
	utils.BitsWrite(bits, 1, 0)
	utils.BitsWrite(bits, 1, 0)
	utils.BitsWrite(bits, 4, 0)
	marker := uint64(0)
	if isMarker {
		marker = uint64(1)
	}
	utils.BitsWrite(bits, 1, marker)
	utils.BitsWrite(bits, 7, uint64(rtpPayloadType))
	utils.BitsWrite(bits, 16, uint64(rtp.cseq))
	utils.BitsWrite(bits, 32, uint64(timestamp))
	utils.BitsWrite(bits, 32, uint64(ssrc))
}

func ParseRTPHeader(rtpData []byte) *RTPHeaderInfo {
	if len(rtpData) < 12 {
		return nil
	}
	headerBytes := rtpData
	rtpHeader := &RTPHeaderInfo{}
	rtpHeader.V = headerBytes[0] >> 6
	rtpHeader.IsPadding = (headerBytes[0] & 0x20) != 0
	rtpHeader.Isextension = (headerBytes[0] & 0x10) != 0
	rtpHeader.CC = headerBytes[0] & 0x0F
	rtpHeader.IsMark = (headerBytes[1] & 0x80) != 0
	rtpHeader.PayloadType = headerBytes[1] & 0x7F
	rtpHeader.Cseq = (uint16(headerBytes[2]) << 8) | uint16(headerBytes[3])
	rtpHeader.Timestamp = (uint32(headerBytes[4]) << 24) | (uint32(headerBytes[5]) << 16) | (uint32(headerBytes[6]) << 8) | (uint32(headerBytes[7]))
	rtpHeader.SSRC = (uint32(headerBytes[8]) << 24) | (uint32(headerBytes[9]) << 16) | (uint32(headerBytes[10]) << 8) | (uint32(headerBytes[11]))

	payloadStart := 12
	if rtpHeader.CC > 0 {
		payloadStart += int(rtpHeader.CC) * 4
	}
	if len(rtpData) < payloadStart {
		return nil
	}
	if rtpHeader.Isextension {
		if len(rtpData) < payloadStart+4 {
			return nil
		}
		rtpHeader.ExtensionDefByProfile = (uint16(headerBytes[payloadStart]) << 8) | uint16(headerBytes[payloadStart+1])
		payloadStart += 2
		rtpHeader.ExtensionLen = (uint16(headerBytes[payloadStart]) << 8) | uint16(headerBytes[payloadStart+1])
		payloadStart += 2
		payloadStart += int(rtpHeader.ExtensionLen) * 4
		if len(rtpData) < payloadStart {
			return nil
		}
	}
	payloadEnd := len(rtpData)
	if rtpHeader.IsPadding {
		rtpHeader.PaddLen = headerBytes[len(rtpData)-1]
		payloadEnd -= int(rtpHeader.PaddLen)
	}
	if payloadStart > payloadEnd {
		return nil
	}
	rtpHeader.Payload = headerBytes[payloadStart:payloadEnd]

	return rtpHeader
}
