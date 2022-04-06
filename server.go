package gb28181

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	. "github.com/logrusorgru/aurora"
	"github.com/pion/rtp"
	"m7s.live/plugin/gb28181/v4/sip"
	"m7s.live/plugin/gb28181/v4/transaction"
)

type Server struct {
	Ignores      map[string]struct{}
	publishers   Publishers
	MediaNetwork string
}

const MaxRegisterCount = 3

func FindChannel(deviceId string, channelId string) (c *Channel) {
	if v, ok := Devices.Load(deviceId); ok {
		d := v.(*Device)
		d.channelMutex.RLock()
		c = d.channelMap[channelId]
		d.channelMutex.RUnlock()
	}
	return
}

type Publishers struct {
	data map[uint32]*GBPublisher
	sync.RWMutex
}

func (p *Publishers) Add(key uint32, pp *GBPublisher) {
	p.Lock()
	p.data[key] = pp
	p.Unlock()
}
func (p *Publishers) Remove(key uint32) {
	p.Lock()
	delete(p.data, key)
	p.Unlock()
}
func (p *Publishers) Get(key uint32) *GBPublisher {
	p.RLock()
	defer p.RUnlock()
	return p.data[key]
}

func (config *GB28181Config) startServer() {
	config.publishers.data = make(map[uint32]*GBPublisher)

	fmt.Println(Green("server gb28181 start at"), BrightBlue(config.SipIP+":"+strconv.Itoa(int(config.SipPort))))

	s := transaction.NewCore(&config.Config)
	s.RegistHandler(sip.REGISTER, config.OnRegister)
	s.RegistHandler(sip.MESSAGE, config.OnMessage)
	s.RegistHandler(sip.BYE, config.onBye)

	//OnStreamClosedHooks.AddHook(func(stream *Stream) {
	//	Devices.Range(func(key, value interface{}) bool {
	//		device:=value.(*Device)
	//		for _,channel := range device.Channels {
	//			if stream.StreamPath == channel.RecordSP {
	//
	//			}
	//		}
	//	})
	//})

	go config.startMediaServer()

	// go queryCatalog(config)
	if config.Username != "" || config.Password != "" {
		go removeBanDevice(config)
	}

	s.StartAndWait()
}

func (config *GB28181Config) startMediaServer() {
	if config.MediaNetwork == "tcp" {
		listenMediaTCP(config)
	} else {
		listenMediaUDP(config)
	}
}

func listenMediaTCP(config *GB28181Config) {
	// for i := uint16(0); i < config.TCPMediaPortNum; i++ {
	// 	addr := ":" + strconv.Itoa(int(config.MediaPort+i))
	// 	go ListenTCP(addr, func(conn net.Conn) {
	// 		var rtpPacket rtp.Packet
	// 		reader := bufio.NewReader(conn)
	// 		lenBuf := make([]byte, 2)
	// 		defer conn.Close()
	// 		var err error
	// 		for err == nil {
	// 			if _, err = io.ReadFull(reader, lenBuf); err != nil {
	// 				return
	// 			}
	// 			ps := make([]byte, BigEndian.Uint16(lenBuf))
	// 			if _, err = io.ReadFull(reader, ps); err != nil {
	// 				return
	// 			}
	// 			if err := rtpPacket.Unmarshal(ps); err != nil {
	// 				Println("gb28181 decode rtp error:", err)
	// 			} else if publisher := publishers.Get(rtpPacket.SSRC); publisher != nil && publisher.Err() == nil {
	// 				publisher.PushPS(&rtpPacket)
	// 			}
	// 		}
	// 	})
	// }
}

func listenMediaUDP(config *GB28181Config) {
	var rtpPacket rtp.Packet
	networkBuffer := 1048576

	addr := config.MediaIP + ":" + strconv.Itoa(int(config.MediaPort))
	mediaAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.ListenUDP("udp", mediaAddr)

	if err != nil {
		fmt.Printf("listen udp %s err: %v", addr, err)
		return
	}
	bufUDP := make([]byte, networkBuffer)
	fmt.Printf("udp server start listen video port[%d]", config.MediaPort)
	defer fmt.Printf("udp server stop listen video port[%d]", config.MediaPort)
	for n, _, err := conn.ReadFromUDP(bufUDP); err == nil; n, _, err = conn.ReadFromUDP(bufUDP) {
		ps := bufUDP[:n]
		if err := rtpPacket.Unmarshal(ps); err != nil {
			fmt.Println("gb28181 decode rtp error:", err)
		}
		if publisher := config.publishers.Get(rtpPacket.SSRC); publisher != nil && publisher.Err() == nil {
			publisher.PushPS(&rtpPacket)
		}
	}
}

// func queryCatalog(config *transaction.Config) {
// 	t := time.NewTicker(time.Duration(config.CatalogInterval) * time.Second)
// 	for range t.C {
// 		Devices.Range(func(key, value interface{}) bool {
// 			device := value.(*Device)
// 			if time.Since(device.UpdateTime) > time.Duration(config.RegisterValidity)*time.Second {
// 				Devices.Delete(key)
// 			} else if device.Channels != nil {
// 				go device.Catalog()
// 			}
// 			return true
// 		})
// 	}
// }

func removeBanDevice(config *GB28181Config) {
	t := time.NewTicker(time.Duration(config.RemoveBanInterval) * time.Second)
	for range t.C {
		DeviceRegisterCount.Range(func(key, value interface{}) bool {
			if value.(int) > MaxRegisterCount {
				DeviceRegisterCount.Delete(key)
			}
			return true
		})
	}
}
