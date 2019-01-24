// Code generated by protoc-gen-go. DO NOT EDIT.
// source: rpc/ircbot.proto

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

type SongAnnouncement struct {
	Song                 *Song    `protobuf:"bytes,1,opt,name=song" json:"song,omitempty"`
	Listeners            int64    `protobuf:"varint,2,opt,name=listeners" json:"listeners,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SongAnnouncement) Reset()         { *m = SongAnnouncement{} }
func (m *SongAnnouncement) String() string { return proto.CompactTextString(m) }
func (*SongAnnouncement) ProtoMessage()    {}
func (*SongAnnouncement) Descriptor() ([]byte, []int) {
	return fileDescriptor_ircbot_54f79e1281b7720c, []int{0}
}
func (m *SongAnnouncement) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SongAnnouncement.Unmarshal(m, b)
}
func (m *SongAnnouncement) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SongAnnouncement.Marshal(b, m, deterministic)
}
func (dst *SongAnnouncement) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SongAnnouncement.Merge(dst, src)
}
func (m *SongAnnouncement) XXX_Size() int {
	return xxx_messageInfo_SongAnnouncement.Size(m)
}
func (m *SongAnnouncement) XXX_DiscardUnknown() {
	xxx_messageInfo_SongAnnouncement.DiscardUnknown(m)
}

var xxx_messageInfo_SongAnnouncement proto.InternalMessageInfo

func (m *SongAnnouncement) GetSong() *Song {
	if m != nil {
		return m.Song
	}
	return nil
}

func (m *SongAnnouncement) GetListeners() int64 {
	if m != nil {
		return m.Listeners
	}
	return 0
}

func init() {
	proto.RegisterType((*SongAnnouncement)(nil), "radio.rpc.SongAnnouncement")
}

func init() { proto.RegisterFile("rpc/ircbot.proto", fileDescriptor_ircbot_54f79e1281b7720c) }

var fileDescriptor_ircbot_54f79e1281b7720c = []byte{
	// 210 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x5c, 0x8f, 0xb1, 0x4b, 0xc4, 0x30,
	0x14, 0x87, 0xa9, 0x27, 0xc2, 0x45, 0xe1, 0xce, 0x4c, 0x47, 0x15, 0x3c, 0x75, 0xb9, 0xe5, 0x12,
	0xa8, 0xab, 0x8b, 0x75, 0x77, 0xa8, 0xb8, 0xb8, 0xa5, 0x69, 0x88, 0xc1, 0xf4, 0xbd, 0xf0, 0x92,
	0x0a, 0xfd, 0xef, 0x25, 0x2d, 0x6a, 0x71, 0xfd, 0xbe, 0xe4, 0xfb, 0xf1, 0xd8, 0x96, 0x82, 0x96,
	0x8e, 0x74, 0x8b, 0x49, 0x04, 0xc2, 0x84, 0x7c, 0x4d, 0xaa, 0x73, 0x28, 0x28, 0xe8, 0xf2, 0x32,
	0xcb, 0x5e, 0x81, 0xb2, 0x86, 0x66, 0x5b, 0x6e, 0x32, 0x4a, 0x63, 0x30, 0x71, 0x06, 0x77, 0x6f,
	0x6c, 0xfb, 0x8a, 0x60, 0x9f, 0x00, 0x70, 0x00, 0x6d, 0x7a, 0x03, 0x89, 0xdf, 0xb3, 0xd3, 0x88,
	0x60, 0x77, 0xc5, 0xbe, 0x38, 0x9c, 0x57, 0x1b, 0xf1, 0x5b, 0x14, 0xf9, 0x69, 0x33, 0x49, 0x7e,
	0xcd, 0xd6, 0xde, 0xc5, 0x64, 0xc0, 0x50, 0xdc, 0x9d, 0xec, 0x8b, 0xc3, 0xaa, 0xf9, 0x03, 0xd5,
	0x33, 0x5b, 0xd5, 0x98, 0xf8, 0x23, 0xbb, 0xf8, 0x29, 0xe7, 0xaf, 0xfc, 0xea, 0x5f, 0x6b, 0x39,
	0x5b, 0x2e, 0x87, 0x5e, 0x06, 0xef, 0xeb, 0xdb, 0xf7, 0x1b, 0xeb, 0xd2, 0xc7, 0xd0, 0x0a, 0x8d,
	0xbd, 0x6c, 0x8e, 0xea, 0xd8, 0x39, 0x94, 0x5f, 0xca, 0x7f, 0x8e, 0xe4, 0x8c, 0xa4, 0xa0, 0xdb,
	0xb3, 0xe9, 0x8a, 0x87, 0xef, 0x00, 0x00, 0x00, 0xff, 0xff, 0x13, 0x05, 0x6b, 0xd4, 0x08, 0x01,
	0x00, 0x00,
}