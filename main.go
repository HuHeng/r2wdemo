package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jiyeyuran/mediasoup-go"
	"github.com/jiyeyuran/mediasoup-go/h264"
	"github.com/q191201771/lal/pkg/avc"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lal/pkg/rtmp"
	"github.com/q191201771/lal/pkg/rtprtcp"
	"github.com/q191201771/naza/pkg/nazalog"
)

var logger = mediasoup.NewLogger("ExampleApp")

var (
	worker          *mediasoup.Worker
	router          *mediasoup.Router
	directTransport *mediasoup.DirectTransport
	addr            string
	localIP         string
)

const (
	VSSRC uint32 = 12345
	ASSRC uint32 = 123456
)

func GetLocalIP() {
	localIP = "127.0.0.1"
	conn, err := net.Dial("udp", "google.com:80")
	if err != nil {
		log.Println(err.Error())
		return
	}
	defer conn.Close()
	localIP = strings.Split(conn.LocalAddr().String(), ":")[0]
	log.Println("local ip: ", localIP)
}

type OpusPacker struct {
}

func (r *OpusPacker) Pack(in []byte, maxSize int) (out [][]byte) {
	if in == nil {
		return [][]byte{}
	}

	o := make([]byte, len(in))
	copy(o, in)
	return [][]byte{o}
}

func init() {
	GetLocalIP()
	var err error
	worker, err = mediasoup.NewWorker()
	if err != nil {
		panic(err)
	}
	worker.On("died", func(err error) {
		logger.Error("%s", err)
	})

	dump, _ := worker.Dump()
	logger.Debug("dump: %+v", dump)

	usage, err := worker.GetResourceUsage()
	if err != nil {
		panic(err)
	}
	data, _ := json.Marshal(usage)
	logger.Debug("usage: %s", data)

	router, err = worker.CreateRouter(mediasoup.RouterOptions{
		MediaCodecs: []*mediasoup.RtpCodecCapability{
			{
				Kind:      "audio",
				MimeType:  "audio/opus",
				ClockRate: 48000,
				Channels:  2,
			},
			{
				Kind:      "video",
				MimeType:  "video/VP8",
				ClockRate: 90000,
			},
			{
				Kind:      "video",
				MimeType:  "video/H264",
				ClockRate: 90000,
				Parameters: mediasoup.RtpCodecSpecificParameters{
					RtpParameter: h264.RtpParameter{
						LevelAsymmetryAllowed: 1,
						PacketizationMode:     1,
						ProfileLevelId:        "42001f",
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	//
	directTransport, err = router.CreateDirectTransport()
	if err != nil {
		panic(err)
	}
}

type State int

const (
	Init State = iota
	WaitingClientCapabilities
	WaitingDtlsParameters
	Playing
	Finish
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
} // use default options

type TransportOption struct {
	ID    string                   `json:"id"`
	IceP  mediasoup.IceParameters  `json:"iceParameters"`
	IceCs []mediasoup.IceCandidate `json:"iceCandidates"`
	DtlsP mediasoup.DtlsParameters `json:"dtlsParameters"`
}

type ServerMessage1 struct {
	Flag                  string                    `json:"flag"`
	VProduceID            string                    `json:"vpid"`
	AProduceID            string                    `json:"apid"`
	RouterRtpCapabilities mediasoup.RtpCapabilities `json:"routerrc"`
	RecvTransOption       TransportOption           `json:"recvtransoption"`
}

type ServerMessage2 struct {
	Flag                   string                  `json:"flag"`
	VConsumerID            string                  `json:"vcid"`
	AConsumerID            string                  `json:"acid"`
	VConsumerRtpParameters mediasoup.RtpParameters `json:"vcrtpp"`
	AConsumerRtpParameters mediasoup.RtpParameters `json:"acrtpp"`
}

type ClientMessage1 struct {
	ClientCapabilities mediasoup.RtpCapabilities `json:"clientcap"`
}

type ClientMessage2 struct {
	DtlsP mediasoup.DtlsParameters `json:"dtlsp"`
}

type Session struct {
	state            State
	conn             *websocket.Conn
	errChan          chan error
	pullResChan      chan error
	rtmpUrl          string
	pullSession      *rtmp.PullSession
	vproducer        *mediasoup.Producer
	aproducer        *mediasoup.Producer
	vconsumer        *mediasoup.Consumer
	aconsumer        *mediasoup.Consumer
	consumeTransport *mediasoup.WebRtcTransport
}

func (s *Session) ProcessMessage() error {
	switch s.state {
	case Init:
		//unmarshal as rtmp url
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			log.Println("read message from ws err: ", err)
			break
		}
		s.rtmpUrl = string(message)
		err = s.CreateProducerAndRtmpSession()
		if err != nil {
			s.state = Finish
			return err
		}
		s.state = WaitingClientCapabilities

	case WaitingClientCapabilities:
		cm1 := &ClientMessage1{}

		err := s.conn.ReadJSON(cm1)
		if err != nil {
			log.Println("read client message err:", err)
			return err
		}

		log.Printf("client message: %+v\n", cm1)
		err = s.ApplyClientMessage1(cm1)
		if err != nil {
			log.Println("apply client 1 message err:", err)
			return err
		}
		s.state = WaitingDtlsParameters

	case WaitingDtlsParameters:
		cm2 := &ClientMessage2{}

		err := s.conn.ReadJSON(cm2)
		if err != nil {
			log.Println("read client message err:", err)
			return err
		}

		log.Printf("client message: %+v\n", cm2)
		err = s.ApplyClientMessage2(cm2)
		if err != nil {
			log.Println("apply client 2 message err:", err)
			return err
		}

		s.state = Playing
	case Playing:
		_, _, err := s.conn.ReadMessage()
		if err != nil {
			log.Println("read message from ws err: ", err)
			return err
		}

	case Finish:
		//do nothing
	}
	return nil
}

func (s *Session) ApplyClientMessage1(cm1 *ClientMessage1) error {
	vconsumer, err := s.consumeTransport.Consume(mediasoup.ConsumerOptions{
		ProducerId:      s.vproducer.Id(),
		RtpCapabilities: cm1.ClientCapabilities,
	})

	if err != nil {
		return err
	}

	s.vconsumer = vconsumer

	aconsumer, err := s.consumeTransport.Consume(mediasoup.ConsumerOptions{
		ProducerId:      s.aproducer.Id(),
		RtpCapabilities: cm1.ClientCapabilities,
	})

	if err != nil {
		return err
	}

	s.aconsumer = aconsumer

	//send consumer option
	sm2 := &ServerMessage2{
		Flag:                   "s2",
		VConsumerID:            vconsumer.Id(),
		AConsumerID:            aconsumer.Id(),
		VConsumerRtpParameters: vconsumer.RtpParameters(),
		AConsumerRtpParameters: aconsumer.RtpParameters(),
	}
	log.Printf("send sm2: %+v\n", sm2)

	err = s.conn.WriteJSON(sm2)
	if err != nil {
		log.Println("write err:", err)
		return err
	}
	return nil
}

func (s *Session) ApplyClientMessage2(cm2 *ClientMessage2) error {
	//transport connect
	err := s.consumeTransport.Connect(mediasoup.TransportConnectOptions{
		DtlsParameters: &cm2.DtlsP,
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *Session) CreateProducerAndRtmpSession() error {
	//producer
	vproducer, err := directTransport.Produce(mediasoup.ProducerOptions{
		Kind: mediasoup.MediaKind_Video,
		RtpParameters: mediasoup.RtpParameters{
			//Mid: "VIDEO",
			Codecs: []*mediasoup.RtpCodecParameters{
				{
					MimeType:    "video/H264",
					PayloadType: 125,
					ClockRate:   90000,
					Parameters: mediasoup.RtpCodecSpecificParameters{
						RtpParameter: h264.RtpParameter{
							LevelAsymmetryAllowed: 1,
							PacketizationMode:     1,
							ProfileLevelId:        "42001f",
						},
					},
				},
			},
			Encodings: []mediasoup.RtpEncodingParameters{
				{
					Ssrc: VSSRC,
				},
			},
		},
	})

	if err != nil {
		log.Println("producer err: ", err)
		return err
	}

	aproducer, err := directTransport.Produce(mediasoup.ProducerOptions{
		Kind: mediasoup.MediaKind_Video,
		RtpParameters: mediasoup.RtpParameters{
			//Mid: "AUDIO",
			Codecs: []*mediasoup.RtpCodecParameters{
				{
					MimeType:    "audio/opus",
					PayloadType: 126,
					ClockRate:   48000,
					Channels:    2,
				},
			},
			Encodings: []mediasoup.RtpEncodingParameters{
				{
					Ssrc: ASSRC,
				},
			},
		},
	})

	if err != nil {
		log.Println("producer err: ", err)
		return err
	}

	s.vproducer = vproducer
	s.aproducer = aproducer

	//pull rtmp and producer.send inject media into mediasoup

	pullSession := rtmp.NewPullSession(func(option *rtmp.PullSessionOption) {
	})

	s.pullSession = pullSession

	//vpacker

	avcPacker := rtprtcp.NewRtpPackerPayloadAvc(func(option *rtprtcp.RtpPackerPayloadAvcHevcOption) {
		option.Typ = rtprtcp.RtpPackerPayloadAvcHevcTypeAnnexb
	})

	vrtpPacker := rtprtcp.NewRtpPacker(avcPacker, 90000, VSSRC)

	//apacket
	opusPacker := &OpusPacker{}
	artpPacker := rtprtcp.NewRtpPacker(opusPacker, 48000, ASSRC)

	prevTimestamp := uint32(0)
	var sps []byte
	var pps []byte
	//only support video
	err = pullSession.Pull(s.rtmpUrl, func(msg base.RtmpMsg) {
		switch msg.Header.MsgTypeId {
		case base.RtmpTypeIdMetadata:
			// noop
			return
		case base.RtmpTypeIdAudio:
			//opus
			if (msg.Payload[0] >> 4) == 13 {
				pkts := artpPacker.Pack(base.AvPacket{
					Timestamp:   msg.Header.TimestampAbs,
					PayloadType: 126,
					Payload:     msg.Payload[1:],
				})

				for _, pkt := range pkts {
					err := aproducer.Send(pkt.Raw)
					if err != nil {
						log.Println("audio send pkt to mediasoup err: ", err)
					}
				}

			}

		case base.RtmpTypeIdVideo:
			codecid := msg.Payload[0] & 0xF
			if codecid != base.RtmpCodecIdAvc {
				log.Printf("invalid codec id since only support avc yet. codecid=%d\n", codecid)
				return
			}
		}

		//nazalog.Tracef("< R len=%d, type=%s, hex=%s", len(msg.Payload), avc.ParseNaluTypeReadable(msg.Payload[5+4]), hex.Dump(nazastring.SubSliceSafety(msg.Payload, 16)))

		timestamp := msg.Header.TimestampAbs

		if msg.IsAvcKeySeqHeader() {
			sps, pps, err = avc.ParseSpsPpsFromSeqHeader(msg.Payload)
			nazalog.Assert(nil, err)
			log.Printf("cache spspps. sps=%d, pps=%d\n", len(sps), len(pps))
			return
		}

		if prevTimestamp == 0 {
			prevTimestamp = timestamp
		}

		var out []byte
		err = avc.IterateNaluAvcc(msg.Payload[5:], func(nal []byte) {
			t := avc.ParseNaluType(nal[0])
			//nazalog.Debugf("iterate nalu. type=%s, len=%d", avc.ParseNaluTypeReadable(nal[0]), len(nal))
			if t == avc.NaluTypeSei {
				return
			}

			if t == avc.NaluTypeIdrSlice {
				out = append(out, avc.NaluStartCode3...)
				out = append(out, sps...)
				out = append(out, avc.NaluStartCode3...)
				out = append(out, pps...)
				//nazalog.Debugf("append spspps. sps=%d, pps=%d", len(sps), len(pps))
			}

			out = append(out, avc.NaluStartCode3...)
			out = append(out, nal...)
		})

		//package rtp, send to mediasoup
		if len(out) != 0 {
			pkts := vrtpPacker.Pack(base.AvPacket{
				Timestamp:   timestamp,
				PayloadType: 125,
				Payload:     out,
			})

			for _, pkt := range pkts {
				err := vproducer.Send(pkt.Raw)
				if err != nil {
					log.Println("send pkt to mediasoup err: ", err)
				}
			}
		}

		prevTimestamp = timestamp
	})
	if err != nil {
		log.Println("pull session error: ", err)
		return err
	}
	go func() {
		err := <-pullSession.WaitChan()
		s.pullResChan <- err
	}()

	//create consumer transport
	recvTransport, err := router.CreateWebRtcTransport(mediasoup.WebRtcTransportOptions{
		ListenIps: []mediasoup.TransportListenIp{
			{Ip: "0.0.0.0", AnnouncedIp: localIP}, // AnnouncedIp is optional
		},
	})

	if err != nil {
		log.Println("create webrtctransport err: ", err)
		return err
	}

	s.consumeTransport = recvTransport

	//send to client
	m := &ServerMessage1{
		Flag:                  "s1",
		VProduceID:            vproducer.Id(),
		AProduceID:            aproducer.Id(),
		RouterRtpCapabilities: router.RtpCapabilities(),
		RecvTransOption: TransportOption{
			ID:    recvTransport.Id(),
			IceP:  recvTransport.IceParameters(),
			IceCs: recvTransport.IceCandidates(),
			DtlsP: recvTransport.DtlsParameters(),
		},
	}

	log.Printf("write %+v\n", m)

	err = s.conn.WriteJSON(m)
	if err != nil {
		log.Println("write err:", err)
		return err
	}

	return nil
}

func NewWebsocketConn(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer c.Close()

	s := &Session{
		state:       Init,
		conn:        c,
		errChan:     make(chan error, 1),
		pullResChan: make(chan error, 1),
	}

	go func() {
		for {
			err = s.ProcessMessage()
			if err != nil {
				break
			}
		}
		s.errChan <- err

	}()

	select {
	case <-s.errChan:
		log.Println("process signaling err: ", err)

	case <-s.pullResChan:
		log.Println("pull rtmp finish: ", err)
	}

	//finish pull session
	if s.pullSession != nil {
		s.pullSession.Dispose()

	}
	if s.vproducer != nil {
		s.vproducer.Close()
	}
	if s.vconsumer != nil {
		s.vconsumer.Close()
	}
	if s.aproducer != nil {
		s.aproducer.Close()
	}
	if s.aconsumer != nil {
		s.aconsumer.Close()
	}
	if s.consumeTransport != nil {
		s.consumeTransport.Close()
	}

	log.Println("finish session: ", s.rtmpUrl)
}

func main() {
	flag.StringVar(&addr, "addr", "localhost:3000", "http service address")
	flag.Parse()
	http.HandleFunc("/ws", NewWebsocketConn)
	log.Fatal(http.ListenAndServe(addr, nil))
}
