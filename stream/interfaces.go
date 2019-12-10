package stream

type Modles interface {
	GetStream(needMediaInfo MediaInfo) *Stream
}

const (
	//video codec
	NoneVcodec = 0
	AnyVcodec  = 1
	H264Codec  = 2
	H265Codec  = 3
	//audio codec
	NoneAcodec = 0
	AnyAcodec  = 1
	AACCodec   = 2
	MP3Codec   = 3
	G711Codec  = 4
)

type MediaInfo struct {
	IsLive          bool
	StreamID        string
	Vcodec          int
	Acodec          int
	ISNeedTranscode bool
	//for playback+
	Speed int
	Url   string
}
