package segy

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

const (
	// TextualHeaderByteLength is the fixed size of a SEG-Y textual header.
	TextualHeaderByteLength = 3200
	// BinaryHeaderByteLength is the fixed size of a SEG-Y binary header.
	BinaryHeaderByteLength = 400
	// TraceHeaderByteLength is the fixed size of a SEG-Y trace header.
	TraceHeaderByteLength = 240
)

// TextualHeaderEncoding identifies the character encoding used by a textual header.
type TextualHeaderEncoding uint8

const (
	TextualHeaderEncodingUnknown TextualHeaderEncoding = iota
	TextualHeaderEncodingEBCDIC
	TextualHeaderEncodingUTF8
)

// TextualHeader represents a 3200-byte SEG-Y character header.
// The payload can be EBCDIC or UTF-8 depending on Encoding.
type TextualHeader struct {
	Encoding TextualHeaderEncoding
	Data     [TextualHeaderByteLength]byte
}

// BinaryHeader represents the 400-byte SEG-Y binary header.
// Numeric fields map to the standard binary header words.
type BinaryHeader struct {
	JobID                    int32
	LineNumber               int32
	ReelNumber               int32
	DataTracesPerEnsemble    uint16
	AuxTracesPerEnsemble     uint16
	SampleIntervalMicrosec   uint16
	SampleIntervalOriginal   uint16
	SamplesPerTrace          uint16
	SamplesPerTraceOriginal  uint16
	DataSampleFormatCode     uint16
	EnsembleFold             uint16
	TraceSortingCode         uint16
	VerticalSumCode          uint16
	SweepFrequencyAtStart    uint16
	SweepFrequencyAtEnd      uint16
	SweepLengthMicrosec      uint16
	SweepTypeCode            uint16
	SweepTraceNumber         uint16
	SweepTaperLengthAtStart  uint16
	SweepTaperLengthAtEnd    uint16
	TaperType                uint16
	CorrelatedDataTraces     uint16
	BinaryGainRecovered      uint16
	AmplitudeRecoveryMethod  uint16
	MeasurementSystem        uint16
	ImpulseSignalPolarity    uint16
	VibratoryPolarityCode    uint16
	SEGYRevision             uint16
	FixedLengthTraceFlag     uint16
	ExtendedTextHeadersCount int16

	// Raw stores the original 400 bytes exactly as read from the file.
	Raw [BinaryHeaderByteLength]byte
}

// TraceHeader represents the 240-byte SEG-Y trace header.
// Numeric fields map to commonly used standard trace header words.
type TraceHeader struct {
	TraceSequenceLine              int32
	TraceSequenceFile              int32
	FieldRecordNumber              int32
	TraceNumberWithinRecord        int32
	EnergySourcePointNumber        int32
	CDP                            int32
	TraceNumberWithinCDP           int32
	EnsembleNumber                 int32
	TraceNumberWithinEnsemble      int32
	TraceIdentificationCode        int16
	TracesSummedVertically         int16
	TracesStackedHorizontally      int16
	DataUseCode                    int16
	SourceReceiverOffset           int32
	ReceiverGroupElevation         int32
	SourceSurfaceElevation         int32
	SourceDepth                    int32
	ReceiverDatumElevation         int32
	SourceDatumElevation           int32
	SourceWaterDepth               int32
	ReceiverWaterDepth             int32
	ElevationScalar                int16
	CoordinateScalar               int16
	SourceX                        int32
	SourceY                        int32
	GroupX                         int32
	GroupY                         int32
	CoordinateUnits                int16
	WeatheringVelocity             int16
	SubweatheringVelocity          int16
	UpholeTimeSourceMillisec       int16
	UpholeTimeGroupMillisec        int16
	SourceStaticCorrectionMillisec int16
	GroupStaticCorrectionMillisec  int16
	TotalStaticAppliedMillisec     int16
	LagTimeA                       int16
	LagTimeB                       int16
	DelayRecordingTime             int16
	MuteTimeStart                  int16
	MuteTimeEnd                    int16
	SamplesInTrace                 uint16
	SampleIntervalMicrosec         uint16
	GainTypeOfFieldInstruments     int16
	InstrumentGainConstant         int16
	InstrumentInitialGain          int16
	Correlated                     int16
	SweepFrequencyAtStart          int16
	SweepFrequencyAtEnd            int16
	SweepLengthMillisec            int16
	SweepType                      int16
	SweepTraceTaperLengthStart     int16
	SweepTraceTaperLengthEnd       int16
	TaperType                      int16
	AliasFilterFrequency           int16
	AliasFilterSlope               int16
	NotchFilterFrequency           int16
	NotchFilterSlope               int16
	LowCutFrequency                int16
	HighCutFrequency               int16
	LowCutSlope                    int16
	HighCutSlope                   int16
	YearDataRecorded               int16
	DayOfYear                      int16
	HourOfDay                      int16
	MinuteOfHour                   int16
	SecondOfMinute                 int16
	TimeBasisCode                  int16
	TraceWeightingFactor           int16
	GeophoneGroupNumberRollSwitch  int16
	GeophoneGroupNumberFirstTrace  int16
	GeophoneGroupNumberLastTrace   int16
	GapSize                        int16
	OverTravel                     int16

	// Raw stores the original 240 bytes exactly as read from the file.
	Raw [TraceHeaderByteLength]byte
}

// File represents an industry-standard SEG-Y file.
// URI stores the source location through Fyne's storage-compatible URI type.
type File struct {
	URI fyne.URI

	TextualHeader         TextualHeader
	BinaryHeader          BinaryHeader
	ExtendedTextualHeader []TextualHeader
	TraceCatalog          *TraceCatalog
}

// TraceHeaderField identifies an indexed trace header field.
type TraceHeaderField string

const (
	TraceHeaderFieldEP     TraceHeaderField = "ep"
	TraceHeaderFieldTRACR  TraceHeaderField = "tracr"
	TraceHeaderFieldTRACL  TraceHeaderField = "tracl"
	TraceHeaderFieldFLDR   TraceHeaderField = "fldr"
	TraceHeaderFieldCDP    TraceHeaderField = "cdp"
	TraceHeaderFieldCDPT   TraceHeaderField = "cdpt"
	TraceHeaderFieldOFFSET TraceHeaderField = "offset"
	TraceHeaderFieldSX     TraceHeaderField = "sx"
	TraceHeaderFieldSY     TraceHeaderField = "sy"
	TraceHeaderFieldGX     TraceHeaderField = "gx"
	TraceHeaderFieldGY     TraceHeaderField = "gy"
)

// TraceIndexEntry stores byte locations and header details for one trace.
type TraceIndexEntry struct {
	TraceNumber      int
	HeaderOffset     int64
	DataOffset       int64
	DataByteLength   int64
	SampleCount      uint16
	SampleFormatCode uint16
	SampleByteLength uint8
	Header           TraceHeader
}

// TraceQueryOperator selects how a TraceQueryPredicate is evaluated.
type TraceQueryOperator uint8

const (
	TraceQueryOpEqual TraceQueryOperator = iota
	TraceQueryOpRange
)

// TraceQueryPredicate defines one query clause for an indexed trace header field.
// For equal comparisons use Value; for range comparisons use MinValue and MaxValue.
type TraceQueryPredicate struct {
	Field    TraceHeaderField
	Operator TraceQueryOperator
	Value    int64
	MinValue int64
	MaxValue int64
}

// HeaderRange stores the minimum and maximum values for one header field.
type HeaderRange struct {
	Min int64
	Max int64
}

// TraceHeaderRangeSummary stores min/max summaries for indexed header fields.
type TraceHeaderRangeSummary struct {
	TraceCount int
	Ranges     map[TraceHeaderField]HeaderRange
}

// String returns one line per header field in the form: field: min=... max=...
func (s TraceHeaderRangeSummary) String() string {
	if len(s.Ranges) == 0 {
		return ""
	}

	var b strings.Builder
	printed := make(map[TraceHeaderField]struct{}, len(s.Ranges))

	for _, field := range indexedTraceHeaderFields() {
		r, ok := s.Ranges[field]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "%s: min=%d max=%d\n", field, r.Min, r.Max)
		printed[field] = struct{}{}
	}

	extraFields := make([]string, 0)
	for field := range s.Ranges {
		if _, ok := printed[field]; ok {
			continue
		}
		extraFields = append(extraFields, string(field))
	}
	sort.Strings(extraFields)
	for _, fieldName := range extraFields {
		field := TraceHeaderField(fieldName)
		r := s.Ranges[field]
		fmt.Fprintf(&b, "%s: min=%d max=%d\n", field, r.Min, r.Max)
	}

	return strings.TrimSuffix(b.String(), "\n")
}

type traceFieldValue struct {
	Value      int64
	EntryIndex int
}

// TraceCatalog provides indexed, database-like access to traces by header values.
type TraceCatalog struct {
	URI     fyne.URI
	Entries []TraceIndexEntry

	equalityIndex map[TraceHeaderField]map[int64][]int
	sortedIndex   map[TraceHeaderField][]traceFieldValue
}

// Open reads a SEG-Y file from a Fyne URI and parses its headers.
func Open(uri fyne.URI) (*File, error) {
	if uri == nil {
		return nil, fmt.Errorf("nil uri")
	}

	fmt.Println("Opening", uri.Path())
	r, err := storage.Reader(uri)
	if err != nil {
		return nil, fmt.Errorf("open segy uri: %w", err)
	}
	defer r.Close()

	return ReadHeaders(uri, r)
}

// ReadHeaders parses textual and binary headers from an io.Reader.
// The reader must be positioned at the beginning of a SEG-Y file.
func ReadHeaders(uri fyne.URI, r io.Reader) (*File, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}

	segyFile := &File{URI: uri}

	textData, err := readExact(r, TextualHeaderByteLength)
	if err != nil {
		return nil, fmt.Errorf("read textual header: %w", err)
	}
	copy(segyFile.TextualHeader.Data[:], textData)
	segyFile.TextualHeader.Encoding = detectTextualHeaderEncoding(textData)

	binaryData, err := readExact(r, BinaryHeaderByteLength)
	if err != nil {
		return nil, fmt.Errorf("read binary header: %w", err)
	}
	bh := parseBinaryHeader(binaryData)
	segyFile.BinaryHeader = bh

	if bh.ExtendedTextHeadersCount > 0 {
		segyFile.ExtendedTextualHeader = make([]TextualHeader, 0, bh.ExtendedTextHeadersCount)
		for i := int16(0); i < bh.ExtendedTextHeadersCount; i++ {
			extData, extErr := readExact(r, TextualHeaderByteLength)
			if extErr != nil {
				return nil, fmt.Errorf("read extended textual header %d: %w", i+1, extErr)
			}

			ext := TextualHeader{Encoding: detectTextualHeaderEncoding(extData)}
			copy(ext.Data[:], extData)
			segyFile.ExtendedTextualHeader = append(segyFile.ExtendedTextualHeader, ext)
		}
	}

	return segyFile, nil
}

func readExact(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func detectTextualHeaderEncoding(data []byte) TextualHeaderEncoding {
	if utf8.Valid(data) && mostlyPrintableUTF8(data) {
		return TextualHeaderEncodingUTF8
	}
	return TextualHeaderEncodingEBCDIC
}

func mostlyPrintableUTF8(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	printable := 0
	for _, b := range data {
		if b == 0 {
			continue
		}
		if b == '\n' || b == '\r' || b == '\t' || (b >= 32 && b <= 126) {
			printable++
		}
	}

	ratio := float64(printable) / float64(len(data))
	return ratio >= 0.75
}

func parseBinaryHeader(data []byte) BinaryHeader {
	bh := BinaryHeader{}
	copy(bh.Raw[:], data)

	bh.JobID = int32(binary.BigEndian.Uint32(data[0:4]))
	bh.LineNumber = int32(binary.BigEndian.Uint32(data[4:8]))
	bh.ReelNumber = int32(binary.BigEndian.Uint32(data[8:12]))
	bh.DataTracesPerEnsemble = binary.BigEndian.Uint16(data[12:14])
	bh.AuxTracesPerEnsemble = binary.BigEndian.Uint16(data[14:16])
	bh.SampleIntervalMicrosec = binary.BigEndian.Uint16(data[16:18])
	bh.SampleIntervalOriginal = binary.BigEndian.Uint16(data[18:20])
	bh.SamplesPerTrace = binary.BigEndian.Uint16(data[20:22])
	bh.SamplesPerTraceOriginal = binary.BigEndian.Uint16(data[22:24])
	bh.DataSampleFormatCode = binary.BigEndian.Uint16(data[24:26])
	bh.EnsembleFold = binary.BigEndian.Uint16(data[26:28])
	bh.TraceSortingCode = binary.BigEndian.Uint16(data[28:30])
	bh.VerticalSumCode = binary.BigEndian.Uint16(data[30:32])
	bh.SweepFrequencyAtStart = binary.BigEndian.Uint16(data[32:34])
	bh.SweepFrequencyAtEnd = binary.BigEndian.Uint16(data[34:36])
	bh.SweepLengthMicrosec = binary.BigEndian.Uint16(data[36:38])
	bh.SweepTypeCode = binary.BigEndian.Uint16(data[38:40])
	bh.SweepTraceNumber = binary.BigEndian.Uint16(data[40:42])
	bh.SweepTaperLengthAtStart = binary.BigEndian.Uint16(data[42:44])
	bh.SweepTaperLengthAtEnd = binary.BigEndian.Uint16(data[44:46])
	bh.TaperType = binary.BigEndian.Uint16(data[46:48])
	bh.CorrelatedDataTraces = binary.BigEndian.Uint16(data[48:50])
	bh.BinaryGainRecovered = binary.BigEndian.Uint16(data[50:52])
	bh.AmplitudeRecoveryMethod = binary.BigEndian.Uint16(data[52:54])
	bh.MeasurementSystem = binary.BigEndian.Uint16(data[54:56])
	bh.ImpulseSignalPolarity = binary.BigEndian.Uint16(data[56:58])
	bh.VibratoryPolarityCode = binary.BigEndian.Uint16(data[58:60])

	// SEG-Y rev and fixed-length fields are in bytes 301-304 in the binary header.
	bh.SEGYRevision = binary.BigEndian.Uint16(data[300:302])
	bh.FixedLengthTraceFlag = binary.BigEndian.Uint16(data[302:304])
	bh.ExtendedTextHeadersCount = int16(binary.BigEndian.Uint16(data[304:306]))

	return bh
}

// ReadTraceHeader reads and parses one 240-byte SEG-Y trace header from a reader.
func ReadTraceHeader(r io.Reader) (TraceHeader, error) {
	traceData, err := readExact(r, TraceHeaderByteLength)
	if err != nil {
		return TraceHeader{}, fmt.Errorf("read trace header: %w", err)
	}
	return parseTraceHeader(traceData), nil
}

func parseTraceHeader(data []byte) TraceHeader {
	th := TraceHeader{}
	copy(th.Raw[:], data)

	th.TraceSequenceLine = int32(binary.BigEndian.Uint32(data[0:4]))
	th.TraceSequenceFile = int32(binary.BigEndian.Uint32(data[4:8]))
	th.FieldRecordNumber = int32(binary.BigEndian.Uint32(data[8:12]))
	th.TraceNumberWithinRecord = int32(binary.BigEndian.Uint32(data[12:16]))
	th.EnergySourcePointNumber = int32(binary.BigEndian.Uint32(data[16:20]))
	th.CDP = int32(binary.BigEndian.Uint32(data[20:24]))
	th.TraceNumberWithinCDP = int32(binary.BigEndian.Uint32(data[24:28]))
	th.EnsembleNumber = th.CDP
	th.TraceNumberWithinEnsemble = th.TraceNumberWithinCDP
	th.TraceIdentificationCode = int16(binary.BigEndian.Uint16(data[28:30]))
	th.TracesSummedVertically = int16(binary.BigEndian.Uint16(data[30:32]))
	th.TracesStackedHorizontally = int16(binary.BigEndian.Uint16(data[32:34]))
	th.DataUseCode = int16(binary.BigEndian.Uint16(data[34:36]))
	th.SourceReceiverOffset = int32(binary.BigEndian.Uint32(data[36:40]))
	th.ReceiverGroupElevation = int32(binary.BigEndian.Uint32(data[40:44]))
	th.SourceSurfaceElevation = int32(binary.BigEndian.Uint32(data[44:48]))
	th.SourceDepth = int32(binary.BigEndian.Uint32(data[48:52]))
	th.ReceiverDatumElevation = int32(binary.BigEndian.Uint32(data[52:56]))
	th.SourceDatumElevation = int32(binary.BigEndian.Uint32(data[56:60]))
	th.SourceWaterDepth = int32(binary.BigEndian.Uint32(data[60:64]))
	th.ReceiverWaterDepth = int32(binary.BigEndian.Uint32(data[64:68]))
	th.ElevationScalar = int16(binary.BigEndian.Uint16(data[68:70]))
	th.CoordinateScalar = int16(binary.BigEndian.Uint16(data[70:72]))
	th.SourceX = int32(binary.BigEndian.Uint32(data[72:76]))
	th.SourceY = int32(binary.BigEndian.Uint32(data[76:80]))
	th.GroupX = int32(binary.BigEndian.Uint32(data[80:84]))
	th.GroupY = int32(binary.BigEndian.Uint32(data[84:88]))
	th.CoordinateUnits = int16(binary.BigEndian.Uint16(data[88:90]))
	th.WeatheringVelocity = int16(binary.BigEndian.Uint16(data[90:92]))
	th.SubweatheringVelocity = int16(binary.BigEndian.Uint16(data[92:94]))
	th.UpholeTimeSourceMillisec = int16(binary.BigEndian.Uint16(data[94:96]))
	th.UpholeTimeGroupMillisec = int16(binary.BigEndian.Uint16(data[96:98]))
	th.SourceStaticCorrectionMillisec = int16(binary.BigEndian.Uint16(data[98:100]))
	th.GroupStaticCorrectionMillisec = int16(binary.BigEndian.Uint16(data[100:102]))
	th.TotalStaticAppliedMillisec = int16(binary.BigEndian.Uint16(data[102:104]))
	th.LagTimeA = int16(binary.BigEndian.Uint16(data[104:106]))
	th.LagTimeB = int16(binary.BigEndian.Uint16(data[106:108]))
	th.DelayRecordingTime = int16(binary.BigEndian.Uint16(data[108:110]))
	th.MuteTimeStart = int16(binary.BigEndian.Uint16(data[110:112]))
	th.MuteTimeEnd = int16(binary.BigEndian.Uint16(data[112:114]))
	th.SamplesInTrace = binary.BigEndian.Uint16(data[114:116])
	th.SampleIntervalMicrosec = binary.BigEndian.Uint16(data[116:118])
	th.GainTypeOfFieldInstruments = int16(binary.BigEndian.Uint16(data[118:120]))
	th.InstrumentGainConstant = int16(binary.BigEndian.Uint16(data[120:122]))
	th.InstrumentInitialGain = int16(binary.BigEndian.Uint16(data[122:124]))
	th.Correlated = int16(binary.BigEndian.Uint16(data[124:126]))
	th.SweepFrequencyAtStart = int16(binary.BigEndian.Uint16(data[126:128]))
	th.SweepFrequencyAtEnd = int16(binary.BigEndian.Uint16(data[128:130]))
	th.SweepLengthMillisec = int16(binary.BigEndian.Uint16(data[130:132]))
	th.SweepType = int16(binary.BigEndian.Uint16(data[132:134]))
	th.SweepTraceTaperLengthStart = int16(binary.BigEndian.Uint16(data[134:136]))
	th.SweepTraceTaperLengthEnd = int16(binary.BigEndian.Uint16(data[136:138]))
	th.TaperType = int16(binary.BigEndian.Uint16(data[138:140]))
	th.AliasFilterFrequency = int16(binary.BigEndian.Uint16(data[140:142]))
	th.AliasFilterSlope = int16(binary.BigEndian.Uint16(data[142:144]))
	th.NotchFilterFrequency = int16(binary.BigEndian.Uint16(data[144:146]))
	th.NotchFilterSlope = int16(binary.BigEndian.Uint16(data[146:148]))
	th.LowCutFrequency = int16(binary.BigEndian.Uint16(data[148:150]))
	th.HighCutFrequency = int16(binary.BigEndian.Uint16(data[150:152]))
	th.LowCutSlope = int16(binary.BigEndian.Uint16(data[152:154]))
	th.HighCutSlope = int16(binary.BigEndian.Uint16(data[154:156]))
	th.YearDataRecorded = int16(binary.BigEndian.Uint16(data[156:158]))
	th.DayOfYear = int16(binary.BigEndian.Uint16(data[158:160]))
	th.HourOfDay = int16(binary.BigEndian.Uint16(data[160:162]))
	th.MinuteOfHour = int16(binary.BigEndian.Uint16(data[162:164]))
	th.SecondOfMinute = int16(binary.BigEndian.Uint16(data[164:166]))
	th.TimeBasisCode = int16(binary.BigEndian.Uint16(data[166:168]))
	th.TraceWeightingFactor = int16(binary.BigEndian.Uint16(data[168:170]))
	th.GeophoneGroupNumberRollSwitch = int16(binary.BigEndian.Uint16(data[170:172]))
	th.GeophoneGroupNumberFirstTrace = int16(binary.BigEndian.Uint16(data[172:174]))
	th.GeophoneGroupNumberLastTrace = int16(binary.BigEndian.Uint16(data[174:176]))
	th.GapSize = int16(binary.BigEndian.Uint16(data[176:178]))
	th.OverTravel = int16(binary.BigEndian.Uint16(data[178:180]))

	return th
}

// BuildTraceCatalog scans all traces and creates query indexes for header fields.
func (f *File) BuildTraceCatalog() (*TraceCatalog, error) {
	if f == nil {
		return nil, fmt.Errorf("nil segy file")
	}
	if f.URI == nil {
		return nil, fmt.Errorf("nil uri")
	}

	r, err := storage.Reader(f.URI)
	if err != nil {
		return nil, fmt.Errorf("open segy uri: %w", err)
	}
	defer r.Close()

	catalog, err := BuildTraceCatalogFromReader(f.URI, r)
	if err != nil {
		return nil, err
	}

	f.TraceCatalog = catalog
	return catalog, nil
}

// BuildTraceCatalogFromReader scans trace headers and payload lengths from a SEG-Y stream.
// The reader must be positioned at byte zero of the SEG-Y file.
func BuildTraceCatalogFromReader(uri fyne.URI, r io.Reader) (*TraceCatalog, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}

	fileHeaders, err := ReadHeaders(uri, r)
	if err != nil {
		return nil, fmt.Errorf("read segy headers: %w", err)
	}

	catalog := &TraceCatalog{
		URI:           uri,
		Entries:       []TraceIndexEntry{},
		equalityIndex: make(map[TraceHeaderField]map[int64][]int),
		sortedIndex:   make(map[TraceHeaderField][]traceFieldValue),
	}

	traceStart := int64(TextualHeaderByteLength + BinaryHeaderByteLength + len(fileHeaders.ExtendedTextualHeader)*TextualHeaderByteLength)
	pos := traceStart
	traceNumber := 0
	for {
		traceHeaderBytes, readErr := readExact(r, TraceHeaderByteLength)
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			if readErr == io.ErrUnexpectedEOF {
				return nil, fmt.Errorf("partial trace header at byte %d: %w", pos, readErr)
			}
			return nil, fmt.Errorf("read trace header at byte %d: %w", pos, readErr)
		}

		th := parseTraceHeader(traceHeaderBytes)
		sampleCount := th.SamplesInTrace
		if sampleCount == 0 {
			sampleCount = fileHeaders.BinaryHeader.SamplesPerTrace
		}

		formatCode := fileHeaders.BinaryHeader.DataSampleFormatCode
		sampleByteLength, sizeErr := sampleByteLength(formatCode)
		if sizeErr != nil {
			return nil, fmt.Errorf("trace %d at byte %d: %w", traceNumber+1, pos, sizeErr)
		}

		dataByteLength := int64(sampleCount) * int64(sampleByteLength)
		if dataByteLength < 0 {
			return nil, fmt.Errorf("trace %d at byte %d has invalid data length", traceNumber+1, pos)
		}

		entry := TraceIndexEntry{
			TraceNumber:      traceNumber + 1,
			HeaderOffset:     pos,
			DataOffset:       pos + TraceHeaderByteLength,
			DataByteLength:   dataByteLength,
			SampleCount:      sampleCount,
			SampleFormatCode: formatCode,
			SampleByteLength: sampleByteLength,
			Header:           th,
		}
		catalog.Entries = append(catalog.Entries, entry)

		if dataByteLength > 0 {
			_, skipErr := io.CopyN(io.Discard, r, dataByteLength)
			if skipErr != nil {
				return nil, fmt.Errorf("skip trace payload at byte %d: %w", entry.DataOffset, skipErr)
			}
		}

		pos += TraceHeaderByteLength + dataByteLength
		traceNumber++
	}

	catalog.buildIndexes()
	return catalog, nil
}

// FindEqual returns all traces where the field exactly matches value.
func (c *TraceCatalog) FindEqual(field TraceHeaderField, value int64) []TraceIndexEntry {
	if c == nil {
		return nil
	}
	byField, ok := c.equalityIndex[field]
	if !ok {
		return nil
	}
	entryIndexes, ok := byField[value]
	if !ok {
		return nil
	}

	results := make([]TraceIndexEntry, 0, len(entryIndexes))
	for _, idx := range entryIndexes {
		results = append(results, c.Entries[idx])
	}
	return results
}

// FindRange returns all traces where minValue <= field <= maxValue.
func (c *TraceCatalog) FindRange(field TraceHeaderField, minValue, maxValue int64) []TraceIndexEntry {
	if c == nil || maxValue < minValue {
		return nil
	}
	values, ok := c.sortedIndex[field]
	if !ok || len(values) == 0 {
		return nil
	}

	start := sort.Search(len(values), func(i int) bool { return values[i].Value >= minValue })
	end := sort.Search(len(values), func(i int) bool { return values[i].Value > maxValue })
	if start >= end {
		return nil
	}

	results := make([]TraceIndexEntry, 0, end-start)
	for _, fv := range values[start:end] {
		results = append(results, c.Entries[fv.EntryIndex])
	}
	return results
}

// FindEnsembleByEP returns all traces for one energy source point number (EP).
func (c *TraceCatalog) FindEnsembleByEP(ep int64) []TraceIndexEntry {
	return c.FindEqual(TraceHeaderFieldEP, ep)
}

// FindByTRACR returns all traces matching a TRACR value (trace sequence in file/reel).
func (c *TraceCatalog) FindByTRACR(tracr int64) []TraceIndexEntry {
	return c.FindEqual(TraceHeaderFieldTRACR, tracr)
}

// HeaderRangeSummary returns min/max values for each indexed header field.
func (c *TraceCatalog) HeaderRangeSummary() TraceHeaderRangeSummary {
	summary := TraceHeaderRangeSummary{
		Ranges: make(map[TraceHeaderField]HeaderRange),
	}
	if c == nil {
		return summary
	}

	summary.TraceCount = len(c.Entries)
	fields := indexedTraceHeaderFields()
	for _, field := range fields {
		values, ok := c.sortedIndex[field]
		if ok && len(values) > 0 {
			summary.Ranges[field] = HeaderRange{
				Min: values[0].Value,
				Max: values[len(values)-1].Value,
			}
			continue
		}

		if len(c.Entries) == 0 {
			continue
		}

		minValue := traceHeaderFieldValue(field, c.Entries[0].Header)
		maxValue := minValue
		for i := 1; i < len(c.Entries); i++ {
			v := traceHeaderFieldValue(field, c.Entries[i].Header)
			if v < minValue {
				minValue = v
			}
			if v > maxValue {
				maxValue = v
			}
		}

		summary.Ranges[field] = HeaderRange{Min: minValue, Max: maxValue}
	}

	return summary
}

// GroupByField groups trace index entries by a supported trace header field.
func GroupByField(entries []TraceIndexEntry, field TraceHeaderField) map[int64][]TraceIndexEntry {
	groups := make(map[int64][]TraceIndexEntry)
	for _, entry := range entries {
		key := traceHeaderFieldValue(field, entry.Header)
		groups[key] = append(groups[key], entry)
	}
	return groups
}

// GroupByEP groups trace index entries by energy source point number (EP).
func GroupByEP(entries []TraceIndexEntry) map[int64][]TraceIndexEntry {
	return GroupByField(entries, TraceHeaderFieldEP)
}

// GroupByCDP groups trace index entries by CDP number.
func GroupByCDP(entries []TraceIndexEntry) map[int64][]TraceIndexEntry {
	return GroupByField(entries, TraceHeaderFieldCDP)
}

// EqualPredicate returns a query predicate for exact field matching.
func EqualPredicate(field TraceHeaderField, value int64) TraceQueryPredicate {
	return TraceQueryPredicate{
		Field:    field,
		Operator: TraceQueryOpEqual,
		Value:    value,
	}
}

// RangePredicate returns a query predicate for inclusive field ranges.
func RangePredicate(field TraceHeaderField, minValue, maxValue int64) TraceQueryPredicate {
	return TraceQueryPredicate{
		Field:    field,
		Operator: TraceQueryOpRange,
		MinValue: minValue,
		MaxValue: maxValue,
	}
}

// FindWhere applies multiple predicates and returns traces matching all clauses.
// With no predicates, all traces are returned.
func (c *TraceCatalog) FindWhere(predicates ...TraceQueryPredicate) ([]TraceIndexEntry, error) {
	if c == nil {
		return nil, fmt.Errorf("nil trace catalog")
	}
	if len(predicates) == 0 {
		results := make([]TraceIndexEntry, len(c.Entries))
		copy(results, c.Entries)
		return results, nil
	}

	initialIndexes, err := c.entryIndexesForPredicate(predicates[0])
	if err != nil {
		return nil, err
	}

	if len(initialIndexes) == 0 {
		return nil, nil
	}

	working := make(map[int]struct{}, len(initialIndexes))
	for _, idx := range initialIndexes {
		working[idx] = struct{}{}
	}

	for _, predicate := range predicates[1:] {
		nextIndexes, nextErr := c.entryIndexesForPredicate(predicate)
		if nextErr != nil {
			return nil, nextErr
		}

		nextSet := make(map[int]struct{}, len(nextIndexes))
		for _, idx := range nextIndexes {
			nextSet[idx] = struct{}{}
		}

		for idx := range working {
			if _, ok := nextSet[idx]; !ok {
				delete(working, idx)
			}
		}

		if len(working) == 0 {
			return nil, nil
		}
	}

	results := make([]TraceIndexEntry, 0, len(working))
	for idx, entry := range c.Entries {
		if _, ok := working[idx]; ok {
			results = append(results, entry)
		}
	}

	return results, nil
}

// ReadTraceAtOffset reads a trace header and payload bytes from a known header offset.
func (f *File) ReadTraceAtOffset(headerOffset int64, sampleCount uint16) (TraceHeader, []byte, error) {
	if f == nil {
		return TraceHeader{}, nil, fmt.Errorf("nil segy file")
	}
	if f.URI == nil {
		return TraceHeader{}, nil, fmt.Errorf("nil uri")
	}
	if headerOffset < 0 {
		return TraceHeader{}, nil, fmt.Errorf("invalid header offset %d", headerOffset)
	}

	r, err := storage.Reader(f.URI)
	if err != nil {
		return TraceHeader{}, nil, fmt.Errorf("open segy uri: %w", err)
	}
	defer r.Close()

	seeker, ok := r.(io.Seeker)
	if !ok {
		return TraceHeader{}, nil, fmt.Errorf("reader for uri does not support random access")
	}

	_, err = seeker.Seek(headerOffset, io.SeekStart)
	if err != nil {
		return TraceHeader{}, nil, fmt.Errorf("seek to trace header: %w", err)
	}

	th, err := ReadTraceHeader(r)
	if err != nil {
		return TraceHeader{}, nil, err
	}

	resolvedSampleCount := sampleCount
	if resolvedSampleCount == 0 {
		resolvedSampleCount = th.SamplesInTrace
	}
	if resolvedSampleCount == 0 {
		resolvedSampleCount = f.BinaryHeader.SamplesPerTrace
	}

	sampleLen, err := sampleByteLength(f.BinaryHeader.DataSampleFormatCode)
	if err != nil {
		return TraceHeader{}, nil, err
	}

	dataLen := int64(resolvedSampleCount) * int64(sampleLen)
	traceData, err := readExact(r, int(dataLen))
	if err != nil {
		return TraceHeader{}, nil, fmt.Errorf("read trace data: %w", err)
	}

	return th, traceData, nil
}

// ReadTraceFromIndex reads one trace referenced by a catalog entry.
func (f *File) ReadTraceFromIndex(entry TraceIndexEntry) (TraceHeader, []byte, error) {
	return f.ReadTraceAtOffset(entry.HeaderOffset, entry.SampleCount)
}

func (c *TraceCatalog) entryIndexesForPredicate(predicate TraceQueryPredicate) ([]int, error) {
	if c == nil {
		return nil, fmt.Errorf("nil trace catalog")
	}

	switch predicate.Operator {
	case TraceQueryOpEqual:
		fieldMap, ok := c.equalityIndex[predicate.Field]
		if !ok {
			return nil, fmt.Errorf("field %q is not indexed", predicate.Field)
		}
		matches := fieldMap[predicate.Value]
		if len(matches) == 0 {
			return nil, nil
		}
		result := make([]int, len(matches))
		copy(result, matches)
		return result, nil

	case TraceQueryOpRange:
		if predicate.MaxValue < predicate.MinValue {
			return nil, fmt.Errorf("invalid range for field %q: max < min", predicate.Field)
		}
		values, ok := c.sortedIndex[predicate.Field]
		if !ok {
			return nil, fmt.Errorf("field %q is not indexed", predicate.Field)
		}
		if len(values) == 0 {
			return nil, nil
		}

		start := sort.Search(len(values), func(i int) bool { return values[i].Value >= predicate.MinValue })
		end := sort.Search(len(values), func(i int) bool { return values[i].Value > predicate.MaxValue })
		if start >= end {
			return nil, nil
		}

		result := make([]int, 0, end-start)
		for _, fv := range values[start:end] {
			result = append(result, fv.EntryIndex)
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unsupported query operator %d", predicate.Operator)
	}
}

func (c *TraceCatalog) buildIndexes() {
	fields := indexedTraceHeaderFields()
	for _, field := range fields {
		if _, ok := c.equalityIndex[field]; !ok {
			c.equalityIndex[field] = make(map[int64][]int)
		}
		if _, ok := c.sortedIndex[field]; !ok {
			c.sortedIndex[field] = make([]traceFieldValue, 0, len(c.Entries))
		}
	}

	for i, entry := range c.Entries {
		for _, field := range fields {
			value := traceHeaderFieldValue(field, entry.Header)
			c.equalityIndex[field][value] = append(c.equalityIndex[field][value], i)
			c.sortedIndex[field] = append(c.sortedIndex[field], traceFieldValue{Value: value, EntryIndex: i})
		}
	}

	for _, field := range fields {
		sort.Slice(c.sortedIndex[field], func(i, j int) bool {
			return c.sortedIndex[field][i].Value < c.sortedIndex[field][j].Value
		})
	}
}

func indexedTraceHeaderFields() []TraceHeaderField {
	return []TraceHeaderField{
		TraceHeaderFieldEP,
		TraceHeaderFieldTRACR,
		TraceHeaderFieldTRACL,
		TraceHeaderFieldFLDR,
		TraceHeaderFieldCDP,
		TraceHeaderFieldCDPT,
		TraceHeaderFieldOFFSET,
		TraceHeaderFieldSX,
		TraceHeaderFieldSY,
		TraceHeaderFieldGX,
		TraceHeaderFieldGY,
	}
}

func traceHeaderFieldValue(field TraceHeaderField, th TraceHeader) int64 {
	switch field {
	case TraceHeaderFieldEP:
		return int64(th.EnergySourcePointNumber)
	case TraceHeaderFieldTRACR:
		return int64(th.TraceSequenceFile)
	case TraceHeaderFieldTRACL:
		return int64(th.TraceSequenceLine)
	case TraceHeaderFieldFLDR:
		return int64(th.FieldRecordNumber)
	case TraceHeaderFieldCDP:
		return int64(th.CDP)
	case TraceHeaderFieldCDPT:
		return int64(th.TraceNumberWithinCDP)
	case TraceHeaderFieldOFFSET:
		return int64(th.SourceReceiverOffset)
	case TraceHeaderFieldSX:
		return int64(th.SourceX)
	case TraceHeaderFieldSY:
		return int64(th.SourceY)
	case TraceHeaderFieldGX:
		return int64(th.GroupX)
	case TraceHeaderFieldGY:
		return int64(th.GroupY)
	default:
		return 0
	}
}

func sampleByteLength(sampleFormatCode uint16) (uint8, error) {
	switch sampleFormatCode {
	case 1:
		return 4, nil // 4-byte IBM float
	case 2:
		return 4, nil // 4-byte two's complement integer
	case 3:
		return 2, nil // 2-byte two's complement integer
	case 5:
		return 4, nil // 4-byte IEEE float
	case 6:
		return 8, nil // 8-byte IEEE float
	case 8:
		return 1, nil // 1-byte two's complement integer
	default:
		return 0, fmt.Errorf("unsupported sample format code %d", sampleFormatCode)
	}
}
