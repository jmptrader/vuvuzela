// Code generated by easyjson for marshaling/unmarshaling. DO NOT EDIT.

package main

import (
	json "encoding/json"
	easyjson "github.com/davidlazar/easyjson"
	jlexer "github.com/davidlazar/easyjson/jlexer"
	jwriter "github.com/davidlazar/easyjson/jwriter"
)

// suppress unused package warning
var (
	_ *json.RawMessage
	_ *jlexer.Lexer
	_ *jwriter.Writer
	_ easyjson.Marshaler
)

func easyjsonDecodeCoordinatorInfo89aae3ef(in *jlexer.Lexer, out *CoordinatorInfo) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeString()
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "CoordinatorKey":
			if in.IsNull() {
				in.Skip()
				out.CoordinatorKey = nil
			} else {
				out.CoordinatorKey = in.BytesReadable()
			}
		case "CoordinatorAddress":
			out.CoordinatorAddress = string(in.String())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjsonEncodeCoordinatorInfo89aae3ef(out *jwriter.Writer, in CoordinatorInfo) {
	out.RawByte('{')
	first := true
	_ = first
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"CoordinatorKey\":")
	out.Base32Bytes(in.CoordinatorKey)
	if !first {
		out.RawByte(',')
	}
	first = false
	out.RawString("\"CoordinatorAddress\":")
	out.String(string(in.CoordinatorAddress))
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v CoordinatorInfo) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonEncodeCoordinatorInfo89aae3ef(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v CoordinatorInfo) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonEncodeCoordinatorInfo89aae3ef(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *CoordinatorInfo) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonDecodeCoordinatorInfo89aae3ef(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *CoordinatorInfo) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonDecodeCoordinatorInfo89aae3ef(l, v)
}