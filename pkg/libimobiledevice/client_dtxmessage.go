package libimobiledevice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/electricbubble/gidevice/pkg/nskeyedarchiver"
)

const (
	_unregistered = "_Golang-iDevice_Unregistered"
	_over         = "_Golang-iDevice_Over"
)

var ocLock sync.Mutex

func newDtxMessageClient(innerConn InnerConn) *dtxMessageClient {
	c := &dtxMessageClient{
		innerConn:         innerConn,
		msgID:             0,
		publishedChannels: make(map[string]int32),
		openedChannels:    make(map[string]uint32),
		toReply:           make(chan *dtxMessageHeaderPacket),

		mu:        sync.Mutex{},
		resultMap: make(map[interface{}]*DTXMessageResult),

		callbackMap:     make(map[string]func(m DTXMessageResult)),
		callbackArgsMap: make(map[string]func(m DTXMessageResult, args ...interface{})), //added
		cbArgsMap:       make(map[string][]interface{}),                                 //added
	}
	c.RegisterCallback(_unregistered, func(m DTXMessageResult) {})
	c.RegisterCallback(_over, func(m DTXMessageResult) {})
	c.ctx, c.cancelFunc = context.WithCancel(context.Background())
	c.startReceive()
	c.startWaitingForReply()
	return c
}

type dtxMessageClient struct {
	innerConn InnerConn
	msgID     uint32

	publishedChannels map[string]int32
	openedChannels    map[string]uint32

	toReply chan *dtxMessageHeaderPacket

	mu        sync.Mutex
	resultMap map[interface{}]*DTXMessageResult

	callbackMap     map[string]func(m DTXMessageResult)
	callbackArgsMap map[string]func(m DTXMessageResult, args ...interface{}) //added
	cbArgsMap       map[string][]interface{}                                 //added

	ctx        context.Context
	cancelFunc context.CancelFunc
}

func (c *dtxMessageClient) SendDTXMessage(selector string, aux []byte, channelCode uint32, expectsReply bool) (msgID uint32, err error) {
	payload := new(dtxMessagePayloadPacket)
	header := &dtxMessageHeaderPacket{
		ExpectsReply: 1,
	}

	flag := 0x1000
	if !expectsReply {
		flag = 0
		header.ExpectsReply = 0
	}

	var sel []byte
	if sel, err = nskeyedarchiver.Marshal(selector); err != nil {
		return 0, err
	}

	if aux == nil {
		aux = make([]byte, 0)
	}

	payload.Flags = uint32(0x2 | flag)
	payload.AuxiliaryLength = uint32(len(aux))
	payload.TotalLength = uint64(len(aux)) + uint64(len(sel))

	header.Magic = 0x1F3D5B79
	header.CB = uint32(unsafe.Sizeof(*header))
	header.FragmentId = 0
	header.FragmentCount = 1
	header.Length = uint32(unsafe.Sizeof(*payload)) + uint32(payload.TotalLength)
	c.msgID++
	header.Identifier = c.msgID
	header.ConversationIndex = 0
	header.ChannelCode = channelCode

	msgPkt := new(dtxMessagePacket)
	msgPkt.Header = header
	msgPkt.Payload = payload
	msgPkt.Aux = aux
	msgPkt.Sel = sel

	raw, err := msgPkt.Pack()
	if err != nil {
		return 0, err
	}

	debugLog(fmt.Sprintf("--> %s\n", msgPkt))
	msgID = header.Identifier
	err = c.innerConn.Write(raw)
	return
}

type UnmarshalError struct { //added
	Data []byte
}

func (c *dtxMessageClient) ReceiveDTXMessage() (result *DTXMessageResult, err error) { //modified
	defer func() {
		err1 := recover()
		if err1 != nil {
			fmt.Println(err1)
		}
	}()
	bufPayload := new(bytes.Buffer)
	var needToReply *dtxMessageHeaderPacket = nil
	header := new(dtxMessageHeaderPacket)

	for {
		header = new(dtxMessageHeaderPacket)

		lenHeader := int(unsafe.Sizeof(*header))
		var bufHeader []byte
		if bufHeader, err = c.innerConn.Read(lenHeader); err != nil {
			return nil, fmt.Errorf("receive: length of DTXMessageHeader: %w", err)
		}

		if header, err = header.unpack(bytes.NewBuffer(bufHeader)); err != nil {
			return nil, fmt.Errorf("receive: DTXMessageHeader unpack: %w", err)
		}

		if header.ExpectsReply == 1 {
			needToReply = header
		}

		if header.Magic != 0x1F3D5B79 {
			return nil, fmt.Errorf("receive: bad magic %x", header.Magic)
		}

		if header.ConversationIndex == 1 {
			if header.Identifier != c.msgID {
				return nil, fmt.Errorf("receive: except identifier %d new identifier %d", c.msgID, header.Identifier)
			}
		} else if header.ConversationIndex == 0 {
			if header.Identifier > c.msgID {
				c.msgID = header.Identifier
			}
		} else {
			return nil, fmt.Errorf("receive: invalid conversationIndex %d", header.ConversationIndex)
		}

		if header.FragmentId == 0 && header.FragmentCount > 1 {
			continue
		}

		var data []byte
		if data, err = c.innerConn.Read(int(header.Length)); err != nil {
			return nil, fmt.Errorf("receive: length of DTXMessageHeader: %w", err)
		}
		bufPayload.Write(data)

		if header.FragmentId == header.FragmentCount-1 {
			break
		}
	}

	rawPayload := bufPayload.Bytes()
	payload := new(dtxMessagePayloadPacket)
	if payload, err = payload.unpack(bufPayload); err != nil {
		return nil, fmt.Errorf("receive: unpack DTXMessagePayload: %w", err)
	}

	payloadSize := uint32(unsafe.Sizeof(*payload))
	objOffset := uint64(payloadSize + payload.AuxiliaryLength)
	objEndIdx := objOffset + (payload.TotalLength - uint64(payload.AuxiliaryLength))
	if objOffset > objEndIdx {
		return nil, fmt.Errorf("invaild payload total length: %v and auxiliary length: %v", payload.TotalLength, payload.AuxiliaryLength)
	}
	var aux, obj []byte
	if len(rawPayload) < int(objOffset) || len(rawPayload) < int(objEndIdx) {
		return nil, fmt.Errorf("receive: uncompleted data: %w", err)
	}
	aux = rawPayload[payloadSize:objOffset]
	obj = rawPayload[objOffset:objEndIdx]

	result = new(DTXMessageResult)

	if len(aux) > 0 {
		if aux, err := UnmarshalAuxBuffer(aux); err != nil {
			return nil, fmt.Errorf("receive: unpack AUX: %w", err)
		} else {
			result.Aux = aux
		}
	}

	if len(obj) > 0 {
		if unmarshalObj, err := NewNSKeyedArchiver().Unmarshal(obj); err != nil {
			result.Obj = UnmarshalError{obj}
		} else {
			result.Obj = unmarshalObj
		}

	}
	channelID := fmt.Sprintf("%d", int64(1<<32)-int64(header.ChannelCode))
	sObj, ok := result.Obj.(string)
	if fn, do := c.callbackArgsMap[channelID]; do {
		fn(*result, c.cbArgsMap[channelID]...)
		return
	} else if fn, do := c.callbackMap[channelID]; do {
		fn(*result)
	} else if fn, do := c.callbackMap[sObj]; do {
		fn(*result)
	} else {
		c.callbackMap[_unregistered](*result)
	}

	if needToReply != nil {
		go func() { c.toReply <- needToReply }()
	} else {
		var sk interface{} = header.Identifier

		if ok && sObj == "_notifyOfPublishedCapabilities:" {
			sk = "_notifyOfPublishedCapabilities:"
		}
		c.mu.Lock()
		c.resultMap[sk] = result
		c.mu.Unlock()

	}

	return
}

func (c *dtxMessageClient) RegisterCallbackArgs(obj string, cb func(m DTXMessageResult, args ...interface{}), args ...interface{}) {
	if obj == _unregistered || obj == _over {
		c.callbackArgsMap[obj] = cb
		c.cbArgsMap[obj] = args
	} else {
		ocLock.Lock()
		channelID, ok := c.openedChannels[obj]
		ocLock.Unlock()
		if ok {
			channel := fmt.Sprintf("%d", int(channelID))
			c.callbackArgsMap[channel] = cb
			c.cbArgsMap[channel] = args
		} else {
			channelID, _ := c.MakeChannel(obj)
			channel := fmt.Sprintf("%d", int(channelID))
			c.callbackArgsMap[channel] = cb
			c.cbArgsMap[channel] = args
		}
	}
}

func (c *dtxMessageClient) Connection() (publishedChannels map[string]int32, err error) {
	args := NewAuxBuffer()
	if err = args.AppendObject(map[string]interface{}{
		"com.apple.private.DTXBlockCompression": uint64(0),
		"com.apple.private.DTXConnection":       uint64(1),
	}); err != nil {
		return nil, fmt.Errorf("connection DTXMessage: %w", err)
	}

	selector := "_notifyOfPublishedCapabilities:"
	if _, err = c.SendDTXMessage(selector, args.Bytes(), 0, false); err != nil {
		return nil, fmt.Errorf("connection send: %w", err)
	}

	var result *DTXMessageResult
	if result, err = c.GetResult(selector); err != nil {
		return nil, fmt.Errorf("connection receive: %w", err)
	}

	if result.Obj.(string) != "_notifyOfPublishedCapabilities:" {
		return nil, fmt.Errorf("connection: response mismatch: %s", result.Obj)
	}

	aux := result.Aux[0].(map[string]interface{})
	for k, v := range aux {
		c.publishedChannels[k] = int32(v.(uint64))
	}

	return c.publishedChannels, nil
}

func (c *dtxMessageClient) MakeChannel(channel string) (id uint32, err error) { //modified
	ocLock.Lock()
	var ok bool
	if id, ok = c.openedChannels[channel]; ok {
		ocLock.Unlock()
		return id, nil
	} else {
		id = uint32(len(c.openedChannels) + 1)
		ocLock.Unlock()
	}
	args := NewAuxBuffer()
	args.AppendInt32(int32(id))
	if err = args.AppendObject(channel); err != nil {
		return 0, fmt.Errorf("make channel DTXMessage: %w", err)
	}

	selector := "_requestChannelWithCode:identifier:"

	var msgID uint32
	if msgID, err = c.SendDTXMessage(selector, args.Bytes(), 0, true); err != nil {
		return 0, fmt.Errorf("make channel send: %w", err)
	}

	if _, err = c.GetResult(msgID); err != nil {
		return 0, fmt.Errorf("make channel receive: %w", err)
	}
	ocLock.Lock()
	c.openedChannels[channel] = id
	ocLock.Unlock()

	return
}

func (c *dtxMessageClient) RegisterCallback(obj string, cb func(m DTXMessageResult)) {
	c.callbackMap[obj] = cb
}

func (c *dtxMessageClient) GetResult(key interface{}) (*DTXMessageResult, error) {
	startTime := time.Now()
	for {
		time.Sleep(100 * time.Millisecond)
		c.mu.Lock()
		if v, ok := c.resultMap[key]; ok {
			delete(c.resultMap, key)
			c.mu.Unlock()
			return v, nil
		} else {
			c.mu.Unlock()
		}
		if elapsed := time.Since(startTime); elapsed > 30*time.Second {
			return nil, fmt.Errorf("dtx: get result: timeout after %v", elapsed)
		}
	}
}

func (c *dtxMessageClient) Close() {
	c.cancelFunc()
	c.innerConn.Close()
}

func (c *dtxMessageClient) startReceive() {
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				if _, err := c.ReceiveDTXMessage(); err != nil {
					debugLog(fmt.Sprintf("dtx: receive: %s", err))
					if strings.Contains(err.Error(), io.EOF.Error()) {
						c.cancelFunc()
						c.callbackMap[_over](DTXMessageResult{})
						break
					}
				}
			}
		}
	}()
}

func (c *dtxMessageClient) startWaitingForReply() {
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case reqHeader := <-c.toReply:
				replyPayload := new(dtxMessagePayloadPacket)
				replyPayload.Flags = 0
				replyPayload.AuxiliaryLength = 0
				replyPayload.TotalLength = 0

				replyHeader := new(dtxMessageHeaderPacket)
				replyHeader.Magic = 0x1F3D5B79
				replyHeader.CB = uint32(unsafe.Sizeof(*replyHeader))
				replyHeader.FragmentId = 0
				replyHeader.FragmentCount = 1
				replyHeader.Length = uint32(unsafe.Sizeof(*replyPayload)) + uint32(replyPayload.TotalLength)
				replyHeader.Identifier = reqHeader.Identifier
				replyHeader.ConversationIndex = reqHeader.ConversationIndex + 1
				replyHeader.ChannelCode = reqHeader.ChannelCode
				replyHeader.ExpectsReply = 0

				replyPkt := new(dtxMessagePacket)
				replyPkt.Header = replyHeader
				replyPkt.Payload = replyPayload
				replyPkt.Aux = nil
				replyPkt.Sel = nil

				raw, err := replyPkt.Pack()
				if err != nil {
					debugLog(fmt.Sprintf("pack: reply DTXMessage: %s", err))
					continue
				}

				if err = c.innerConn.Write(raw); err != nil {
					debugLog(fmt.Sprintf("send: reply DTXMessage: %s", err))
					continue
				}
			}
		}
	}()
}

type DTXMessageResult struct {
	Obj interface{}
	Aux []interface{}
}
