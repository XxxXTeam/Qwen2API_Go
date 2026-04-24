package ssxmod

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	customBase64Chars = "DGi0YA7BemWnQjCl4_bR3f8SKIF9tUz/xhr2oEOgPpac=61ZqwTudLkM5vHyNXsVJ"
	refreshInterval   = 15 * time.Minute
)

type Manager struct {
	mu        sync.RWMutex
	itna      string
	itna2     string
	timestamp int64
}

func NewManager() *Manager {
	m := &Manager{}
	m.refreshLocked()
	return m
}

func (m *Manager) Get() (string, string) {
	m.mu.RLock()
	valid := m.itna != "" && m.itna2 != "" && time.Since(time.UnixMilli(m.timestamp)) < refreshInterval
	if valid {
		itna := m.itna
		itna2 := m.itna2
		m.mu.RUnlock()
		return itna, itna2
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.itna == "" || m.itna2 == "" || time.Since(time.UnixMilli(m.timestamp)) >= refreshInterval {
		m.refreshLocked()
	}
	return m.itna, m.itna2
}

func (m *Manager) refreshLocked() {
	fields := generateFingerprintFields()
	processed := processFields(fields)

	m.itna = "1-" + customEncode(strings.Join(processed, "^"), true)
	m.itna2 = "1-" + customEncode(strings.Join([]string{
		processed[0],
		processed[1],
		processed[23],
		"0", "", "0", "", "", "0",
		"0", "0",
		processed[32],
		processed[33],
		"0", "0", "0", "0", "0",
	}, "^"), true)
	m.timestamp = time.Now().UnixMilli()
}

func generateFingerprintFields() []string {
	return []string{
		generateDeviceID(),
		"websdk-2.3.15d",
		"1765348410850",
		"91",
		"1|15",
		"zh-CN",
		"-480",
		"16705151|12791",
		"1470|956|283|797|158|0|1470|956|1470|798|0|0",
		"5",
		"MacIntel",
		"10",
		"ANGLE (Apple, ANGLE Metal Renderer: Apple M4, Unspecified Version)|Google Inc. (Apple)",
		"30|30",
		"0",
		"28",
		fmt.Sprintf("5|%d", randomHash()),
		fmt.Sprintf("%d", randomHash()),
		fmt.Sprintf("%d", randomHash()),
		"1",
		"0",
		"1",
		"0",
		"P",
		"0",
		"0",
		"0",
		"416",
		"Google Inc.",
		"8",
		"-1|0|0|0|0",
		fmt.Sprintf("%d", randomHash()),
		"11",
		fmt.Sprintf("%d", time.Now().UnixMilli()),
		fmt.Sprintf("%d", randomHash()),
		"0",
		fmt.Sprintf("%d", rand.Intn(91)+10),
	}
}

func processFields(fields []string) []string {
	processed := append([]string(nil), fields...)
	replaceSplitHash(processed, 16)
	replaceFullHash(processed, 17)
	replaceFullHash(processed, 18)
	replaceFullHash(processed, 31)
	replaceFullHash(processed, 34)
	processed[36] = fmt.Sprintf("%d", rand.Intn(91)+10)
	processed[33] = fmt.Sprintf("%d", time.Now().UnixMilli())
	return processed
}

func replaceSplitHash(fields []string, idx int) {
	parts := strings.Split(fields[idx], "|")
	if len(parts) == 2 {
		fields[idx] = parts[0] + "|" + fmt.Sprintf("%d", randomHash())
	}
}

func replaceFullHash(fields []string, idx int) {
	fields[idx] = fmt.Sprintf("%d", randomHash())
}

func generateDeviceID() string {
	const chars = "0123456789abcdef"
	var builder strings.Builder
	builder.Grow(20)
	for i := 0; i < 20; i++ {
		builder.WriteByte(chars[rand.Intn(len(chars))])
	}
	return builder.String()
}

func randomHash() uint32 {
	return rand.Uint32()
}

func customEncode(data string, urlSafe bool) string {
	compressed := lzwCompress(data, 6, func(index int) byte {
		return customBase64Chars[index]
	})
	if urlSafe {
		return compressed
	}
	switch len(compressed) % 4 {
	case 1:
		return compressed + "==="
	case 2:
		return compressed + "=="
	case 3:
		return compressed + "="
	default:
		return compressed
	}
}

func lzwCompress(data string, bits int, charFunc func(index int) byte) string {
	if data == "" {
		return ""
	}

	dict := map[string]int{}
	dictToCreate := map[string]bool{}
	w := ""
	enlargeIn := 2
	dictSize := 3
	numBits := 2
	result := make([]byte, 0, len(data))
	value := 0
	position := 0

	writeBit := func(bit int) {
		value = (value << 1) | bit
		if position == bits-1 {
			position = 0
			result = append(result, charFunc(value))
			value = 0
		} else {
			position++
		}
	}

	writeCharBits := func(charCode int, count int) {
		for i := 0; i < count; i++ {
			writeBit(charCode & 1)
			charCode >>= 1
		}
	}

	flushCreated := func(token string) {
		if token == "" {
			return
		}
		runes := []rune(token)
		first := int(runes[0])
		if first < 256 {
			for i := 0; i < numBits; i++ {
				writeBit(0)
			}
			writeCharBits(first, 8)
		} else {
			writeBit(1)
			for i := 1; i < numBits; i++ {
				writeBit(0)
			}
			writeCharBits(first, 16)
		}
		enlargeIn--
		if enlargeIn == 0 {
			enlargeIn = int(math.Pow(2, float64(numBits)))
			numBits++
		}
		delete(dictToCreate, token)
	}

	flushCode := func(code int) {
		writeCharBits(code, numBits)
		enlargeIn--
		if enlargeIn == 0 {
			enlargeIn = int(math.Pow(2, float64(numBits)))
			numBits++
		}
	}

	for _, r := range data {
		c := string(r)
		if _, ok := dict[c]; !ok {
			dict[c] = dictSize
			dictSize++
			dictToCreate[c] = true
		}

		wc := w + c
		if _, ok := dict[wc]; ok {
			w = wc
			continue
		}

		if dictToCreate[w] {
			flushCreated(w)
		} else {
			flushCode(dict[w])
		}

		dict[wc] = dictSize
		dictSize++
		w = c
	}

	if w != "" {
		if dictToCreate[w] {
			flushCreated(w)
		} else {
			flushCode(dict[w])
		}
	}

	writeCharBits(2, numBits)
	for {
		value <<= 1
		if position == bits-1 {
			result = append(result, charFunc(value))
			break
		}
		position++
	}

	return string(result)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
