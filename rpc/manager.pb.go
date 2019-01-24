// Code generated by protoc-gen-go. DO NOT EDIT.
// source: rpc/manager.proto

package rpc // import "github.com/R-a-dio/valkyrie/rpc"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type StatusRequest struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StatusRequest) Reset()         { *m = StatusRequest{} }
func (m *StatusRequest) String() string { return proto.CompactTextString(m) }
func (*StatusRequest) ProtoMessage()    {}
func (*StatusRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{0}
}
func (m *StatusRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StatusRequest.Unmarshal(m, b)
}
func (m *StatusRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StatusRequest.Marshal(b, m, deterministic)
}
func (dst *StatusRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StatusRequest.Merge(dst, src)
}
func (m *StatusRequest) XXX_Size() int {
	return xxx_messageInfo_StatusRequest.Size(m)
}
func (m *StatusRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_StatusRequest.DiscardUnknown(m)
}

var xxx_messageInfo_StatusRequest proto.InternalMessageInfo

type StatusResponse struct {
	User                 *User         `protobuf:"bytes,1,opt,name=user" json:"user,omitempty"`
	Song                 *Song         `protobuf:"bytes,2,opt,name=song" json:"song,omitempty"`
	ListenerInfo         *ListenerInfo `protobuf:"bytes,3,opt,name=listener_info,json=listenerInfo" json:"listener_info,omitempty"`
	Thread               *Thread       `protobuf:"bytes,4,opt,name=thread" json:"thread,omitempty"`
	BotConfig            *BotConfig    `protobuf:"bytes,5,opt,name=bot_config,json=botConfig" json:"bot_config,omitempty"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *StatusResponse) Reset()         { *m = StatusResponse{} }
func (m *StatusResponse) String() string { return proto.CompactTextString(m) }
func (*StatusResponse) ProtoMessage()    {}
func (*StatusResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{1}
}
func (m *StatusResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StatusResponse.Unmarshal(m, b)
}
func (m *StatusResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StatusResponse.Marshal(b, m, deterministic)
}
func (dst *StatusResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StatusResponse.Merge(dst, src)
}
func (m *StatusResponse) XXX_Size() int {
	return xxx_messageInfo_StatusResponse.Size(m)
}
func (m *StatusResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_StatusResponse.DiscardUnknown(m)
}

var xxx_messageInfo_StatusResponse proto.InternalMessageInfo

func (m *StatusResponse) GetUser() *User {
	if m != nil {
		return m.User
	}
	return nil
}

func (m *StatusResponse) GetSong() *Song {
	if m != nil {
		return m.Song
	}
	return nil
}

func (m *StatusResponse) GetListenerInfo() *ListenerInfo {
	if m != nil {
		return m.ListenerInfo
	}
	return nil
}

func (m *StatusResponse) GetThread() *Thread {
	if m != nil {
		return m.Thread
	}
	return nil
}

func (m *StatusResponse) GetBotConfig() *BotConfig {
	if m != nil {
		return m.BotConfig
	}
	return nil
}

type Song struct {
	// song identifier
	Id int32 `protobuf:"varint,1,opt,name=id" json:"id,omitempty"`
	// short metadata
	Metadata string `protobuf:"bytes,2,opt,name=metadata" json:"metadata,omitempty"`
	// song start time in unix epoch
	StartTime int64 `protobuf:"varint,3,opt,name=start_time,json=startTime" json:"start_time,omitempty"`
	// song end time in unix epoch, can be inaccurate
	EndTime int64 `protobuf:"varint,4,opt,name=end_time,json=endTime" json:"end_time,omitempty"`
	// database track identifier
	TrackId int32 `protobuf:"varint,5,opt,name=track_id,json=trackId" json:"track_id,omitempty"`
	// last time this song was played
	LastPlayed           int64    `protobuf:"varint,6,opt,name=last_played,json=lastPlayed" json:"last_played,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Song) Reset()         { *m = Song{} }
func (m *Song) String() string { return proto.CompactTextString(m) }
func (*Song) ProtoMessage()    {}
func (*Song) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{2}
}
func (m *Song) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Song.Unmarshal(m, b)
}
func (m *Song) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Song.Marshal(b, m, deterministic)
}
func (dst *Song) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Song.Merge(dst, src)
}
func (m *Song) XXX_Size() int {
	return xxx_messageInfo_Song.Size(m)
}
func (m *Song) XXX_DiscardUnknown() {
	xxx_messageInfo_Song.DiscardUnknown(m)
}

var xxx_messageInfo_Song proto.InternalMessageInfo

func (m *Song) GetId() int32 {
	if m != nil {
		return m.Id
	}
	return 0
}

func (m *Song) GetMetadata() string {
	if m != nil {
		return m.Metadata
	}
	return ""
}

func (m *Song) GetStartTime() int64 {
	if m != nil {
		return m.StartTime
	}
	return 0
}

func (m *Song) GetEndTime() int64 {
	if m != nil {
		return m.EndTime
	}
	return 0
}

func (m *Song) GetTrackId() int32 {
	if m != nil {
		return m.TrackId
	}
	return 0
}

func (m *Song) GetLastPlayed() int64 {
	if m != nil {
		return m.LastPlayed
	}
	return 0
}

type Thread struct {
	// thread string, most of the time an URL
	Thread               string   `protobuf:"bytes,1,opt,name=thread" json:"thread,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Thread) Reset()         { *m = Thread{} }
func (m *Thread) String() string { return proto.CompactTextString(m) }
func (*Thread) ProtoMessage()    {}
func (*Thread) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{3}
}
func (m *Thread) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Thread.Unmarshal(m, b)
}
func (m *Thread) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Thread.Marshal(b, m, deterministic)
}
func (dst *Thread) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Thread.Merge(dst, src)
}
func (m *Thread) XXX_Size() int {
	return xxx_messageInfo_Thread.Size(m)
}
func (m *Thread) XXX_DiscardUnknown() {
	xxx_messageInfo_Thread.DiscardUnknown(m)
}

var xxx_messageInfo_Thread proto.InternalMessageInfo

func (m *Thread) GetThread() string {
	if m != nil {
		return m.Thread
	}
	return ""
}

type BotConfig struct {
	RequestsEnabled      bool     `protobuf:"varint,1,opt,name=requests_enabled,json=requestsEnabled" json:"requests_enabled,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *BotConfig) Reset()         { *m = BotConfig{} }
func (m *BotConfig) String() string { return proto.CompactTextString(m) }
func (*BotConfig) ProtoMessage()    {}
func (*BotConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{4}
}
func (m *BotConfig) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BotConfig.Unmarshal(m, b)
}
func (m *BotConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BotConfig.Marshal(b, m, deterministic)
}
func (dst *BotConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BotConfig.Merge(dst, src)
}
func (m *BotConfig) XXX_Size() int {
	return xxx_messageInfo_BotConfig.Size(m)
}
func (m *BotConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_BotConfig.DiscardUnknown(m)
}

var xxx_messageInfo_BotConfig proto.InternalMessageInfo

func (m *BotConfig) GetRequestsEnabled() bool {
	if m != nil {
		return m.RequestsEnabled
	}
	return false
}

type User struct {
	// user identifier
	Id int32 `protobuf:"varint,1,opt,name=id" json:"id,omitempty"`
	// user nickname, this is only a display-name
	Nickname string `protobuf:"bytes,2,opt,name=nickname" json:"nickname,omitempty"`
	// indicates if this user is a robot or not
	IsRobot              bool     `protobuf:"varint,3,opt,name=is_robot,json=isRobot" json:"is_robot,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *User) Reset()         { *m = User{} }
func (m *User) String() string { return proto.CompactTextString(m) }
func (*User) ProtoMessage()    {}
func (*User) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{5}
}
func (m *User) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_User.Unmarshal(m, b)
}
func (m *User) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_User.Marshal(b, m, deterministic)
}
func (dst *User) XXX_Merge(src proto.Message) {
	xxx_messageInfo_User.Merge(dst, src)
}
func (m *User) XXX_Size() int {
	return xxx_messageInfo_User.Size(m)
}
func (m *User) XXX_DiscardUnknown() {
	xxx_messageInfo_User.DiscardUnknown(m)
}

var xxx_messageInfo_User proto.InternalMessageInfo

func (m *User) GetId() int32 {
	if m != nil {
		return m.Id
	}
	return 0
}

func (m *User) GetNickname() string {
	if m != nil {
		return m.Nickname
	}
	return ""
}

func (m *User) GetIsRobot() bool {
	if m != nil {
		return m.IsRobot
	}
	return false
}

type ListenerInfo struct {
	Listeners            int64    `protobuf:"varint,1,opt,name=listeners" json:"listeners,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListenerInfo) Reset()         { *m = ListenerInfo{} }
func (m *ListenerInfo) String() string { return proto.CompactTextString(m) }
func (*ListenerInfo) ProtoMessage()    {}
func (*ListenerInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_manager_2dfa9ac0354f91c1, []int{6}
}
func (m *ListenerInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListenerInfo.Unmarshal(m, b)
}
func (m *ListenerInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListenerInfo.Marshal(b, m, deterministic)
}
func (dst *ListenerInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListenerInfo.Merge(dst, src)
}
func (m *ListenerInfo) XXX_Size() int {
	return xxx_messageInfo_ListenerInfo.Size(m)
}
func (m *ListenerInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_ListenerInfo.DiscardUnknown(m)
}

var xxx_messageInfo_ListenerInfo proto.InternalMessageInfo

func (m *ListenerInfo) GetListeners() int64 {
	if m != nil {
		return m.Listeners
	}
	return 0
}

func init() {
	proto.RegisterType((*StatusRequest)(nil), "radio.rpc.StatusRequest")
	proto.RegisterType((*StatusResponse)(nil), "radio.rpc.StatusResponse")
	proto.RegisterType((*Song)(nil), "radio.rpc.Song")
	proto.RegisterType((*Thread)(nil), "radio.rpc.Thread")
	proto.RegisterType((*BotConfig)(nil), "radio.rpc.BotConfig")
	proto.RegisterType((*User)(nil), "radio.rpc.User")
	proto.RegisterType((*ListenerInfo)(nil), "radio.rpc.ListenerInfo")
}

func init() { proto.RegisterFile("rpc/manager.proto", fileDescriptor_manager_2dfa9ac0354f91c1) }

var fileDescriptor_manager_2dfa9ac0354f91c1 = []byte{
	// 529 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x54, 0xc1, 0x6e, 0xd3, 0x40,
	0x10, 0x55, 0xd2, 0x34, 0x89, 0xa7, 0x69, 0x43, 0x57, 0x08, 0x5c, 0x0b, 0xd4, 0x62, 0x2e, 0x54,
	0xd0, 0x44, 0xb4, 0x12, 0x07, 0x04, 0x97, 0x56, 0x1c, 0x2a, 0x51, 0x09, 0xad, 0xcb, 0x85, 0x8b,
	0xb5, 0xf6, 0x4e, 0xd2, 0x55, 0xec, 0x5d, 0xb3, 0xbb, 0x41, 0xea, 0x1f, 0xf1, 0x45, 0x7c, 0x0b,
	0x47, 0xe4, 0xb5, 0x9d, 0x9a, 0x24, 0xbd, 0x79, 0xde, 0x7b, 0x63, 0xcd, 0x9b, 0x79, 0x36, 0x1c,
	0xea, 0x22, 0x9d, 0xe6, 0x4c, 0xb2, 0x39, 0xea, 0x49, 0xa1, 0x95, 0x55, 0xc4, 0xd3, 0x8c, 0x0b,
	0x35, 0xd1, 0x45, 0x1a, 0x8e, 0x61, 0x3f, 0xb2, 0xcc, 0x2e, 0x0d, 0xc5, 0x9f, 0x4b, 0x34, 0x36,
	0xfc, 0xdb, 0x81, 0x83, 0x06, 0x31, 0x85, 0x92, 0x06, 0xc9, 0x6b, 0xe8, 0x2d, 0x0d, 0x6a, 0xbf,
	0x73, 0xd2, 0x79, 0xb3, 0x77, 0x3e, 0x9e, 0xac, 0xba, 0x27, 0xdf, 0x0d, 0x6a, 0xea, 0xc8, 0x52,
	0x64, 0x94, 0x9c, 0xfb, 0xdd, 0x0d, 0x51, 0xa4, 0xe4, 0x9c, 0x3a, 0x92, 0x7c, 0x82, 0xfd, 0x4c,
	0x18, 0x8b, 0x12, 0x75, 0x2c, 0xe4, 0x4c, 0xf9, 0x3b, 0x4e, 0xfd, 0xbc, 0xa5, 0xfe, 0x5a, 0xf3,
	0xd7, 0x72, 0xa6, 0xe8, 0x28, 0x6b, 0x55, 0xe4, 0x14, 0xfa, 0xf6, 0x4e, 0x23, 0xe3, 0x7e, 0xcf,
	0xb5, 0x1d, 0xb6, 0xda, 0x6e, 0x1d, 0x41, 0x6b, 0x01, 0xb9, 0x00, 0x48, 0x94, 0x8d, 0x53, 0x25,
	0x67, 0x62, 0xee, 0xef, 0x3a, 0xf9, 0xd3, 0x96, 0xfc, 0x52, 0xd9, 0x2b, 0xc7, 0x51, 0x2f, 0x69,
	0x1e, 0xc3, 0xdf, 0x1d, 0xe8, 0x95, 0xc3, 0x92, 0x03, 0xe8, 0x0a, 0xee, 0xec, 0xee, 0xd2, 0xae,
	0xe0, 0x24, 0x80, 0x61, 0x8e, 0x96, 0x71, 0x66, 0x99, 0xf3, 0xe7, 0xd1, 0x55, 0x4d, 0x5e, 0x02,
	0x18, 0xcb, 0xb4, 0x8d, 0xad, 0xc8, 0xd1, 0xf9, 0xd9, 0xa1, 0x9e, 0x43, 0x6e, 0x45, 0x8e, 0xe4,
	0x08, 0x86, 0x28, 0x79, 0x45, 0xf6, 0x1c, 0x39, 0x40, 0xc9, 0x1b, 0xca, 0x6a, 0x96, 0x2e, 0x62,
	0xc1, 0xdd, 0x84, 0xbb, 0x74, 0xe0, 0xea, 0x6b, 0x4e, 0x8e, 0x61, 0x2f, 0x63, 0xc6, 0xc6, 0x45,
	0xc6, 0xee, 0x91, 0xfb, 0x7d, 0xd7, 0x08, 0x25, 0xf4, 0xcd, 0x21, 0xe1, 0x09, 0xf4, 0x2b, 0xc7,
	0xe4, 0xd9, 0x6a, 0x29, 0x1d, 0x37, 0x59, 0x5d, 0x85, 0x1f, 0xc0, 0x5b, 0x99, 0x24, 0xa7, 0xf0,
	0x44, 0x57, 0xf7, 0x35, 0x31, 0x4a, 0x96, 0x64, 0x58, 0xc9, 0x87, 0x74, 0xdc, 0xe0, 0x5f, 0x2a,
	0x38, 0xbc, 0x81, 0x5e, 0x79, 0xd5, 0x6d, 0x3b, 0x90, 0x22, 0x5d, 0x48, 0x96, 0x63, 0xb3, 0x83,
	0xa6, 0x2e, 0x9d, 0x08, 0x13, 0x6b, 0x95, 0x28, 0xeb, 0x36, 0x30, 0xa4, 0x03, 0x61, 0x68, 0x59,
	0x86, 0xef, 0x60, 0xd4, 0xbe, 0x28, 0x79, 0x01, 0x5e, 0x73, 0x53, 0xe3, 0xde, 0xbe, 0x43, 0x1f,
	0x80, 0xf3, 0x3f, 0x5d, 0x18, 0xdc, 0x54, 0x51, 0x25, 0x9f, 0xa1, 0x5f, 0xe5, 0x90, 0xf8, 0xed,
	0x30, 0xb5, 0xc3, 0x1a, 0x1c, 0x6d, 0x61, 0xea, 0xd0, 0xbe, 0x85, 0x41, 0x84, 0xd6, 0x59, 0x59,
	0x4f, 0x6c, 0xb0, 0x0e, 0xd4, 0x62, 0x77, 0xfb, 0xf5, 0xe4, 0x06, 0xeb, 0x00, 0xf9, 0x08, 0xa3,
	0x08, 0xed, 0xc3, 0x72, 0xb7, 0xe6, 0x2a, 0xd8, 0x8a, 0x92, 0xf7, 0xe0, 0x45, 0x68, 0xeb, 0xd3,
	0x6d, 0xe6, 0x37, 0xd8, 0x84, 0xc8, 0x15, 0x8c, 0x23, 0xb4, 0xff, 0x2d, 0xf1, 0xb1, 0xef, 0x25,
	0x78, 0x8c, 0xb8, 0x7c, 0xf5, 0xe3, 0x78, 0x2e, 0xec, 0xdd, 0x32, 0x99, 0xa4, 0x2a, 0x9f, 0xd2,
	0x33, 0x76, 0xc6, 0x85, 0x9a, 0xfe, 0x62, 0xd9, 0xe2, 0x5e, 0x0b, 0x9c, 0xea, 0x22, 0x4d, 0xfa,
	0xee, 0xdf, 0x70, 0xf1, 0x2f, 0x00, 0x00, 0xff, 0xff, 0xf6, 0xd7, 0x5b, 0xae, 0x30, 0x04, 0x00,
	0x00,
}