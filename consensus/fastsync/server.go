package fastsync

import (
	"bytes"
	"io"
	"log"
	"sync"

	"github.com/icon-project/goloop/common/codec"
	"github.com/icon-project/goloop/module"
)

const (
	configChunkSize = 1024 * 10
)

type MessageItem struct {
	pi module.ProtocolInfo
	b  []byte
}

type speer struct {
	id       module.PeerID
	msgCh    chan MessageItem
	cancelCh chan struct{}
}

type server struct {
	sync.Mutex
	nm    NetworkManager
	ph    module.ProtocolHandler
	bm    BlockManager
	peers []*speer

	running bool
}

func newServer(nm NetworkManager, ph module.ProtocolHandler, bm BlockManager) *server {
	s := &server{
		nm: nm,
		ph: ph,
		bm: bm,
	}
	return s
}

func (s *server) start() {
	s.Lock()
	defer s.Unlock()

	if !s.running {
		s.running = true
		pids := s.nm.GetPeers()
		for _, id := range pids {
			s._addPeer(id)
		}
	}
}

func (s *server) _addPeer(id module.PeerID) {
	speer := &speer{
		id:       id,
		msgCh:    make(chan MessageItem),
		cancelCh: make(chan struct{}),
	}
	s.peers = append(s.peers, speer)
	h := newSConHandler(speer.msgCh, speer.cancelCh, speer.id, s.ph, s.bm)
	go h.handle()
}

func (s *server) stop() {
	s.Lock()
	defer s.Unlock()

	if s.running {
		s.running = false
		for _, p := range s.peers {
			close(p.cancelCh)
		}
		s.peers = nil
	}
}

func (s *server) onJoin(id module.PeerID) {
	s.Lock()
	defer s.Unlock()

	if !s.running {
		return
	}
	for _, p := range s.peers {
		if p.id.Equal(id) {
			return
		}
	}
	s._addPeer(id)
}

func (s *server) onLeave(id module.PeerID) {
	s.Lock()
	defer s.Unlock()

	if !s.running {
		return
	}
	for i, p := range s.peers {
		if p.id.Equal(id) {
			last := len(s.peers) - 1
			s.peers[i] = s.peers[last]
			s.peers[last] = nil
			s.peers = s.peers[:last]
			close(p.cancelCh)
			return
		}
	}
}

func (s *server) onReceive(pi module.ProtocolInfo, b []byte, id module.PeerID) {
	s.Lock()
	defer s.Unlock()

	if !s.running {
		return
	}
	for _, p := range s.peers {
		if p.id.Equal(id) {
			if pi == protoCancelAllBlockRequests {
				p.cancelCh <- struct{}{}
			}
			p.msgCh <- MessageItem{pi, b}
		}
	}
}

type sconHandler struct {
	msgCh    <-chan MessageItem
	cancelCh <-chan struct{}
	id       module.PeerID
	ph       module.ProtocolHandler
	bm       BlockManager

	nextItems []*BlockRequest
	buf       *bytes.Buffer
	requestID uint32
	nextMsgPI module.ProtocolInfo
	nextMsg   []byte
}

func newSConHandler(
	msgCh <-chan MessageItem,
	cancelCh <-chan struct{},
	id module.PeerID,
	ph module.ProtocolHandler,
	bm BlockManager,
) *sconHandler {
	h := &sconHandler{
		msgCh:    msgCh,
		cancelCh: cancelCh,
		id:       id,
		ph:       ph,
		bm:       bm,
	}
	return h
}

func (h *sconHandler) cancelAllRequests() {
	h.buf = nil
	h.nextItems = nil
	for {
		msgItem := <-h.msgCh
		if msgItem.pi == protoCancelAllBlockRequests {
			break
		}
	}
}

func (h *sconHandler) updateCurrentTask() {
	if len(h.nextItems) == 0 {
		return
	}
	ni := h.nextItems[0]
	copy(h.nextItems, h.nextItems[1:])
	h.nextItems = h.nextItems[:len(h.nextItems)-1]
	h.requestID = ni.RequestID
	blk, err := h.bm.GetBlockByHeight(ni.Height)
	nblk, err2 := h.bm.GetBlockByHeight(ni.Height + 1)
	if err != nil || err2 != nil {
		h.nextMsgPI = protoBlockMetadata
		h.nextMsg = codec.MustMarshalToBytes(&BlockMetadata{
			RequestID:   ni.RequestID,
			BlockLength: -1,
			VoteList:    nil,
		})
		h.buf = nil
		return
	}
	h.buf = bytes.NewBuffer(nil)
	blk.MarshalHeader(h.buf)
	blk.MarshalBody(h.buf)
	h.nextMsgPI = protoBlockMetadata
	h.nextMsg = codec.MustMarshalToBytes(&BlockMetadata{
		RequestID:   ni.RequestID,
		BlockLength: int32(h.buf.Len()),
		VoteList:    nblk.Votes().Bytes(),
	})
}

func (h *sconHandler) updateNextMsg() {
	if h.nextMsg != nil {
		return
	}
	if h.buf == nil {
		h.updateCurrentTask()
		return
	}
	chunk := make([]byte, configChunkSize)
	var data []byte
	n, err := h.buf.Read(chunk)
	if n > 0 {
		data = chunk[:n]
	} else if n == 0 && err == io.EOF {
		h.updateCurrentTask()
		return
	} else {
		// n==0 && err!=io.EOF
		log.Panicf("n=%d, err=%+v\n", n, err)
	}
	var msg BlockData
	msg.RequestID = h.requestID
	msg.Data = data
	h.nextMsgPI = protoBlockData
	h.nextMsg = codec.MustMarshalToBytes(&msg)
}

func (h *sconHandler) handle() {
loop:
	for {
		select {
		case _, more := <-h.cancelCh:
			if !more {
				break loop
			}
			h.cancelAllRequests()
			continue loop
		default:
		}

		h.updateNextMsg()
		if h.nextMsg != nil {
			err := h.ph.Unicast(h.nextMsgPI, h.nextMsg, h.id)
			if err == nil {
				// TODO: refactor
				h.nextMsg = nil
				h.updateNextMsg()
			} else {
				log.Printf("error=%+v\n", err)
			}
		}

		// if packet is dropped too much, use ticker to slow down sending
		if len(h.nextMsg) > 0 {
			select {
			case _, more := <-h.cancelCh:
				if !more {
					break loop
				}
				h.cancelAllRequests()
				continue loop
			case msgItem := <-h.msgCh:
				if msgItem.pi == protoBlockRequest {
					var msg BlockRequest
					_, err := codec.UnmarshalFromBytes(msgItem.b, &msg)
					if err != nil {
						// TODO log
						continue loop
					}
					h.nextItems = append(h.nextItems, &msg)
				}
			default:
			}
		} else {
			select {
			case _, more := <-h.cancelCh:
				if !more {
					break loop
				}
				h.cancelAllRequests()
				continue
			case msgItem := <-h.msgCh:
				if msgItem.pi == protoBlockRequest {
					var msg BlockRequest
					_, err := codec.UnmarshalFromBytes(msgItem.b, &msg)
					if err != nil {
						// TODO log
						continue loop
					}
					h.nextItems = append(h.nextItems, &msg)
				}
			}
		}
	}
}