package store

// A minimal protobuf wire-format reader. Antigravity stores its conversation data as protobuf
// blobs without a published schema, so fields are read by number (see antigravity.go).

type pbField struct {
	num   int
	wire  int
	val   uint64 // varint / fixed values
	bytes []byte // length-delimited values (strings, nested messages)
}

// pbParse splits b into its top-level fields; ok is false if b isn't valid protobuf.
func pbParse(b []byte) (fields []pbField, ok bool) {
	for i := 0; i < len(b); {
		key, n := pbVarint(b[i:])
		if n == 0 {
			return nil, false
		}
		i += n
		f := pbField{num: int(key >> 3), wire: int(key & 7)}
		if f.num == 0 {
			return nil, false
		}
		switch f.wire {
		case 0:
			v, n := pbVarint(b[i:])
			if n == 0 {
				return nil, false
			}
			f.val, i = v, i+n
		case 1:
			if i+8 > len(b) {
				return nil, false
			}
			for k := 7; k >= 0; k-- {
				f.val = f.val<<8 | uint64(b[i+k])
			}
			i += 8
		case 2:
			l, n := pbVarint(b[i:])
			if n == 0 || i+n+int(l) > len(b) || int(l) < 0 {
				return nil, false
			}
			i += n
			f.bytes, i = b[i:i+int(l)], i+int(l)
		case 5:
			if i+4 > len(b) {
				return nil, false
			}
			for k := 3; k >= 0; k-- {
				f.val = f.val<<8 | uint64(b[i+k])
			}
			i += 4
		default:
			return nil, false
		}
		fields = append(fields, f)
	}
	return fields, true
}

func pbVarint(b []byte) (v uint64, n int) {
	for shift := uint(0); n < len(b) && n < 10; shift += 7 {
		c := b[n]
		n++
		v |= uint64(c&0x7f) << shift
		if c < 0x80 {
			return v, n
		}
	}
	return 0, 0
}

// pbMsg: parsed fields with lookup helpers; missing fields read as zero values.
type pbMsg []pbField

func pbDecode(b []byte) pbMsg {
	f, _ := pbParse(b)
	return f
}

func (m pbMsg) first(num int) (pbField, bool) {
	for _, f := range m {
		if f.num == num {
			return f, true
		}
	}
	return pbField{}, false
}

// int: a varint field.
func (m pbMsg) int(num int) int64 {
	f, ok := m.first(num)
	if !ok || f.wire != 0 {
		return 0
	}
	return int64(f.val)
}

// str: a length-delimited field as a string.
func (m pbMsg) str(num int) string {
	f, ok := m.first(num)
	if !ok || f.wire != 2 {
		return ""
	}
	return string(f.bytes)
}

// msg: a nested message.
func (m pbMsg) msg(num int) pbMsg {
	f, ok := m.first(num)
	if !ok || f.wire != 2 {
		return nil
	}
	return pbDecode(f.bytes)
}
