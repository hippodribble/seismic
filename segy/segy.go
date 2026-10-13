package segy

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
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
	// DefaultMaxImageSamples is a conservative safety limit for ensemble image creation.
	DefaultMaxImageSamples int64 = 100_000_000
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
	// ByteOrder controls parsing and decoding of binary values.
	// SEG-Y is typically big-endian, but some files are little-endian.
	ByteOrder binary.ByteOrder

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

// TraceSortOrder controls ascending or descending sorting.
type TraceSortOrder uint8

const (
	TraceSortAscending TraceSortOrder = iota
	TraceSortDescending
)

// TraceSortKey defines one field and direction for multi-key sorting.
type TraceSortKey struct {
	Field TraceHeaderField
	Order TraceSortOrder
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

// EnsembleSamples contains decoded trace samples prepared for image rendering.
type EnsembleSamples struct {
	Traces            [][]float32
	TraceCount        int
	SamplesPerTrace   int
	TotalImageSamples int64
}

// EnsembleImageOptions controls scaling and clipping when mapping data to palette indices.
type EnsembleImageOptions struct {
	Palette color.Palette

	// PercentileClip clips low/high tails equally when ZeroCentered is false.
	PercentileClip float64

	// ZeroCentered maps value 0 to ZeroIndex and applies separate clipping to
	// negative and positive values.
	ZeroCentered bool
	ZeroIndex    uint8

	// NegativePercentileClip clips the negative tail (towards lower values).
	// PositivePercentileClip clips the positive tail (towards higher values).
	// When <= 0, PercentileClip is used.
	NegativePercentileClip float64
	PositivePercentileClip float64

	// Gain applies optional time-variant gain before clipping and palette mapping.
	Gain GainOptions
}

// GainMode identifies the gain transform applied before rendering.
type GainMode uint8

const (
	GainModeNone GainMode = iota
	GainModeTSquared
	GainModeAGC
)

// GainOptions controls optional time-variant gain transforms.
type GainOptions struct {
	Mode GainMode

	// TSquaredScale controls t-squared gain strength in gain = 1 + TSquaredScale*t^2.
	// When <= 0, a scale of 1 is used.
	TSquaredScale float64

	// SampleIntervalSec is optional for t-squared gain.
	// When > 0, time is computed as sampleIndex * SampleIntervalSec.
	// When <= 0, sample index is used as the time axis.
	SampleIntervalSec float64

	// AGCWindowSamples is the AGC RMS window length in samples.
	// When <= 0, a default window of 101 samples is used.
	AGCWindowSamples int

	// AGCFloor is the minimum RMS denominator to avoid unstable AGC scaling.
	// When <= 0, a default floor of 1e-6 is used.
	AGCFloor float64
}

// EnsembleImageStats reports scaling bounds used to create a palette image.
type EnsembleImageStats struct {
	LowerClip float32
	UpperClip float32
	ZeroIndex uint8
	Width     int
	Height    int
}

// TooManySamplesError indicates that an ensemble exceeds the configured threshold.
type TooManySamplesError struct {
	Requested int64
	Threshold int64
}

func (e TooManySamplesError) Error() string {
	return fmt.Sprintf("ensemble image requires %d samples, exceeds threshold %d", e.Requested, e.Threshold)
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

// OpenWithByteOrder reads a SEG-Y file from a URI using the provided byte order.
func OpenWithByteOrder(uri fyne.URI, byteOrder binary.ByteOrder) (*File, error) {
	if uri == nil {
		return nil, fmt.Errorf("nil uri")
	}

	r, err := storage.Reader(uri)
	if err != nil {
		return nil, fmt.Errorf("open segy uri: %w", err)
	}
	defer r.Close()

	return ReadHeadersWithByteOrder(uri, r, byteOrder)
}

// ReadHeaders parses textual and binary headers from an io.Reader.
// The reader must be positioned at the beginning of a SEG-Y file.
func ReadHeaders(uri fyne.URI, r io.Reader) (*File, error) {
	return ReadHeadersWithByteOrder(uri, r, binary.BigEndian)
}

// ReadHeadersWithByteOrder parses textual and binary headers using the provided byte order.
// The reader must be positioned at the beginning of a SEG-Y file.
func ReadHeadersWithByteOrder(uri fyne.URI, r io.Reader, byteOrder binary.ByteOrder) (*File, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}
	if byteOrder == nil {
		return nil, fmt.Errorf("nil byte order")
	}

	segyFile := &File{URI: uri, ByteOrder: byteOrder}

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
	bh := parseBinaryHeader(binaryData, byteOrder)
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

func parseBinaryHeader(data []byte, byteOrder binary.ByteOrder) BinaryHeader {
	bh := BinaryHeader{}
	copy(bh.Raw[:], data)

	bh.JobID = int32(byteOrder.Uint32(data[0:4]))
	bh.LineNumber = int32(byteOrder.Uint32(data[4:8]))
	bh.ReelNumber = int32(byteOrder.Uint32(data[8:12]))
	bh.DataTracesPerEnsemble = byteOrder.Uint16(data[12:14])
	bh.AuxTracesPerEnsemble = byteOrder.Uint16(data[14:16])
	bh.SampleIntervalMicrosec = byteOrder.Uint16(data[16:18])
	bh.SampleIntervalOriginal = byteOrder.Uint16(data[18:20])
	bh.SamplesPerTrace = byteOrder.Uint16(data[20:22])
	bh.SamplesPerTraceOriginal = byteOrder.Uint16(data[22:24])
	bh.DataSampleFormatCode = byteOrder.Uint16(data[24:26])
	bh.EnsembleFold = byteOrder.Uint16(data[26:28])
	bh.TraceSortingCode = byteOrder.Uint16(data[28:30])
	bh.VerticalSumCode = byteOrder.Uint16(data[30:32])
	bh.SweepFrequencyAtStart = byteOrder.Uint16(data[32:34])
	bh.SweepFrequencyAtEnd = byteOrder.Uint16(data[34:36])
	bh.SweepLengthMicrosec = byteOrder.Uint16(data[36:38])
	bh.SweepTypeCode = byteOrder.Uint16(data[38:40])
	bh.SweepTraceNumber = byteOrder.Uint16(data[40:42])
	bh.SweepTaperLengthAtStart = byteOrder.Uint16(data[42:44])
	bh.SweepTaperLengthAtEnd = byteOrder.Uint16(data[44:46])
	bh.TaperType = byteOrder.Uint16(data[46:48])
	bh.CorrelatedDataTraces = byteOrder.Uint16(data[48:50])
	bh.BinaryGainRecovered = byteOrder.Uint16(data[50:52])
	bh.AmplitudeRecoveryMethod = byteOrder.Uint16(data[52:54])
	bh.MeasurementSystem = byteOrder.Uint16(data[54:56])
	bh.ImpulseSignalPolarity = byteOrder.Uint16(data[56:58])
	bh.VibratoryPolarityCode = byteOrder.Uint16(data[58:60])

	// SEG-Y rev and fixed-length fields are in bytes 301-304 in the binary header.
	bh.SEGYRevision = byteOrder.Uint16(data[300:302])
	bh.FixedLengthTraceFlag = byteOrder.Uint16(data[302:304])
	bh.ExtendedTextHeadersCount = int16(byteOrder.Uint16(data[304:306]))

	return bh
}

// ReadTraceHeader reads and parses one 240-byte SEG-Y trace header from a reader.
func ReadTraceHeader(r io.Reader) (TraceHeader, error) {
	return ReadTraceHeaderWithByteOrder(r, binary.BigEndian)
}

// ReadTraceHeaderWithByteOrder reads and parses one 240-byte SEG-Y trace header using byte order.
func ReadTraceHeaderWithByteOrder(r io.Reader, byteOrder binary.ByteOrder) (TraceHeader, error) {
	traceData, err := readExact(r, TraceHeaderByteLength)
	if err != nil {
		return TraceHeader{}, fmt.Errorf("read trace header: %w", err)
	}
	return parseTraceHeader(traceData, byteOrder), nil
}

func parseTraceHeader(data []byte, byteOrder binary.ByteOrder) TraceHeader {
	th := TraceHeader{}
	copy(th.Raw[:], data)

	th.TraceSequenceLine = int32(byteOrder.Uint32(data[0:4]))
	th.TraceSequenceFile = int32(byteOrder.Uint32(data[4:8]))
	th.FieldRecordNumber = int32(byteOrder.Uint32(data[8:12]))
	th.TraceNumberWithinRecord = int32(byteOrder.Uint32(data[12:16]))
	th.EnergySourcePointNumber = int32(byteOrder.Uint32(data[16:20]))
	th.CDP = int32(byteOrder.Uint32(data[20:24]))
	th.TraceNumberWithinCDP = int32(byteOrder.Uint32(data[24:28]))
	th.EnsembleNumber = th.CDP
	th.TraceNumberWithinEnsemble = th.TraceNumberWithinCDP
	th.TraceIdentificationCode = int16(byteOrder.Uint16(data[28:30]))
	th.TracesSummedVertically = int16(byteOrder.Uint16(data[30:32]))
	th.TracesStackedHorizontally = int16(byteOrder.Uint16(data[32:34]))
	th.DataUseCode = int16(byteOrder.Uint16(data[34:36]))
	th.SourceReceiverOffset = int32(byteOrder.Uint32(data[36:40]))
	th.ReceiverGroupElevation = int32(byteOrder.Uint32(data[40:44]))
	th.SourceSurfaceElevation = int32(byteOrder.Uint32(data[44:48]))
	th.SourceDepth = int32(byteOrder.Uint32(data[48:52]))
	th.ReceiverDatumElevation = int32(byteOrder.Uint32(data[52:56]))
	th.SourceDatumElevation = int32(byteOrder.Uint32(data[56:60]))
	th.SourceWaterDepth = int32(byteOrder.Uint32(data[60:64]))
	th.ReceiverWaterDepth = int32(byteOrder.Uint32(data[64:68]))
	th.ElevationScalar = int16(byteOrder.Uint16(data[68:70]))
	th.CoordinateScalar = int16(byteOrder.Uint16(data[70:72]))
	th.SourceX = int32(byteOrder.Uint32(data[72:76]))
	th.SourceY = int32(byteOrder.Uint32(data[76:80]))
	th.GroupX = int32(byteOrder.Uint32(data[80:84]))
	th.GroupY = int32(byteOrder.Uint32(data[84:88]))
	th.CoordinateUnits = int16(byteOrder.Uint16(data[88:90]))
	th.WeatheringVelocity = int16(byteOrder.Uint16(data[90:92]))
	th.SubweatheringVelocity = int16(byteOrder.Uint16(data[92:94]))
	th.UpholeTimeSourceMillisec = int16(byteOrder.Uint16(data[94:96]))
	th.UpholeTimeGroupMillisec = int16(byteOrder.Uint16(data[96:98]))
	th.SourceStaticCorrectionMillisec = int16(byteOrder.Uint16(data[98:100]))
	th.GroupStaticCorrectionMillisec = int16(byteOrder.Uint16(data[100:102]))
	th.TotalStaticAppliedMillisec = int16(byteOrder.Uint16(data[102:104]))
	th.LagTimeA = int16(byteOrder.Uint16(data[104:106]))
	th.LagTimeB = int16(byteOrder.Uint16(data[106:108]))
	th.DelayRecordingTime = int16(byteOrder.Uint16(data[108:110]))
	th.MuteTimeStart = int16(byteOrder.Uint16(data[110:112]))
	th.MuteTimeEnd = int16(byteOrder.Uint16(data[112:114]))
	th.SamplesInTrace = byteOrder.Uint16(data[114:116])
	th.SampleIntervalMicrosec = byteOrder.Uint16(data[116:118])
	th.GainTypeOfFieldInstruments = int16(byteOrder.Uint16(data[118:120]))
	th.InstrumentGainConstant = int16(byteOrder.Uint16(data[120:122]))
	th.InstrumentInitialGain = int16(byteOrder.Uint16(data[122:124]))
	th.Correlated = int16(byteOrder.Uint16(data[124:126]))
	th.SweepFrequencyAtStart = int16(byteOrder.Uint16(data[126:128]))
	th.SweepFrequencyAtEnd = int16(byteOrder.Uint16(data[128:130]))
	th.SweepLengthMillisec = int16(byteOrder.Uint16(data[130:132]))
	th.SweepType = int16(byteOrder.Uint16(data[132:134]))
	th.SweepTraceTaperLengthStart = int16(byteOrder.Uint16(data[134:136]))
	th.SweepTraceTaperLengthEnd = int16(byteOrder.Uint16(data[136:138]))
	th.TaperType = int16(byteOrder.Uint16(data[138:140]))
	th.AliasFilterFrequency = int16(byteOrder.Uint16(data[140:142]))
	th.AliasFilterSlope = int16(byteOrder.Uint16(data[142:144]))
	th.NotchFilterFrequency = int16(byteOrder.Uint16(data[144:146]))
	th.NotchFilterSlope = int16(byteOrder.Uint16(data[146:148]))
	th.LowCutFrequency = int16(byteOrder.Uint16(data[148:150]))
	th.HighCutFrequency = int16(byteOrder.Uint16(data[150:152]))
	th.LowCutSlope = int16(byteOrder.Uint16(data[152:154]))
	th.HighCutSlope = int16(byteOrder.Uint16(data[154:156]))
	th.YearDataRecorded = int16(byteOrder.Uint16(data[156:158]))
	th.DayOfYear = int16(byteOrder.Uint16(data[158:160]))
	th.HourOfDay = int16(byteOrder.Uint16(data[160:162]))
	th.MinuteOfHour = int16(byteOrder.Uint16(data[162:164]))
	th.SecondOfMinute = int16(byteOrder.Uint16(data[164:166]))
	th.TimeBasisCode = int16(byteOrder.Uint16(data[166:168]))
	th.TraceWeightingFactor = int16(byteOrder.Uint16(data[168:170]))
	th.GeophoneGroupNumberRollSwitch = int16(byteOrder.Uint16(data[170:172]))
	th.GeophoneGroupNumberFirstTrace = int16(byteOrder.Uint16(data[172:174]))
	th.GeophoneGroupNumberLastTrace = int16(byteOrder.Uint16(data[174:176]))
	th.GapSize = int16(byteOrder.Uint16(data[176:178]))
	th.OverTravel = int16(byteOrder.Uint16(data[178:180]))

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

	byteOrder := f.ByteOrder
	if byteOrder == nil {
		byteOrder = binary.BigEndian
	}

	catalog, err := BuildTraceCatalogFromReaderWithByteOrder(f.URI, r, byteOrder)
	if err != nil {
		return nil, err
	}

	f.TraceCatalog = catalog
	return catalog, nil
}

// BuildTraceCatalogFromReader scans trace headers and payload lengths from a SEG-Y stream.
// The reader must be positioned at byte zero of the SEG-Y file.
func BuildTraceCatalogFromReader(uri fyne.URI, r io.Reader) (*TraceCatalog, error) {
	return BuildTraceCatalogFromReaderWithByteOrder(uri, r, binary.BigEndian)
}

// BuildTraceCatalogFromReaderWithByteOrder scans trace headers and payload lengths using byte order.
// The reader must be positioned at byte zero of the SEG-Y file.
func BuildTraceCatalogFromReaderWithByteOrder(uri fyne.URI, r io.Reader, byteOrder binary.ByteOrder) (*TraceCatalog, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}
	if byteOrder == nil {
		return nil, fmt.Errorf("nil byte order")
	}

	fileHeaders, err := ReadHeadersWithByteOrder(uri, r, byteOrder)
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

		th := parseTraceHeader(traceHeaderBytes, byteOrder)
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

// FindRangeSorted returns range-matching traces sorted by one or more keys.
func (c *TraceCatalog) FindRangeSorted(field TraceHeaderField, minValue, maxValue int64, keys ...TraceSortKey) ([]TraceIndexEntry, error) {
	results := c.FindRange(field, minValue, maxValue)
	if len(results) == 0 {
		return results, nil
	}
	if err := SortTraceIndexEntries(results, keys...); err != nil {
		return nil, err
	}
	return results, nil
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

// FindWhereSorted applies predicates and then sorts matching traces by one or more keys.
func (c *TraceCatalog) FindWhereSorted(predicates []TraceQueryPredicate, keys ...TraceSortKey) ([]TraceIndexEntry, error) {
	results, err := c.FindWhere(predicates...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return results, nil
	}
	if err := SortTraceIndexEntries(results, keys...); err != nil {
		return nil, err
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

	byteOrder := f.ByteOrder
	if byteOrder == nil {
		byteOrder = binary.BigEndian
	}

	th, err := ReadTraceHeaderWithByteOrder(r, byteOrder)
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

// DecodeTraceSamples converts raw trace bytes into float32 values.
// Decoding uses SEG-Y sample format code and the provided byte order.
func DecodeTraceSamples(traceData []byte, sampleFormatCode uint16, byteOrder binary.ByteOrder) ([]float32, error) {
	if byteOrder == nil {
		return nil, fmt.Errorf("nil byte order")
	}

	sampleLen, err := sampleByteLength(sampleFormatCode)
	if err != nil {
		return nil, err
	}
	if int(sampleLen) == 0 {
		return nil, fmt.Errorf("invalid sample length for format code %d", sampleFormatCode)
	}
	if len(traceData)%int(sampleLen) != 0 {
		return nil, fmt.Errorf("trace data length %d is not a multiple of sample size %d", len(traceData), sampleLen)
	}

	count := len(traceData) / int(sampleLen)
	out := make([]float32, count)

	switch sampleFormatCode {
	case 1:
		for i := 0; i < count; i++ {
			raw := byteOrder.Uint32(traceData[i*4 : i*4+4])
			out[i] = ibm32ToFloat32(raw)
		}
	case 2:
		for i := 0; i < count; i++ {
			v := int32(byteOrder.Uint32(traceData[i*4 : i*4+4]))
			out[i] = float32(v)
		}
	case 3:
		for i := 0; i < count; i++ {
			v := int16(byteOrder.Uint16(traceData[i*2 : i*2+2]))
			out[i] = float32(v)
		}
	case 5:
		for i := 0; i < count; i++ {
			raw := byteOrder.Uint32(traceData[i*4 : i*4+4])
			out[i] = math.Float32frombits(raw)
		}
	case 6:
		for i := 0; i < count; i++ {
			raw := byteOrder.Uint64(traceData[i*8 : i*8+8])
			out[i] = float32(math.Float64frombits(raw))
		}
	case 8:
		for i := 0; i < count; i++ {
			out[i] = float32(int8(traceData[i]))
		}
	default:
		return nil, fmt.Errorf("unsupported sample format code %d", sampleFormatCode)
	}

	return out, nil
}

// DecodeTraceData converts raw trace bytes to float32 using this file's format and byte order.
func (f *File) DecodeTraceData(traceData []byte) ([]float32, error) {
	if f == nil {
		return nil, fmt.Errorf("nil segy file")
	}
	byteOrder := f.ByteOrder
	if byteOrder == nil {
		byteOrder = binary.BigEndian
	}
	return DecodeTraceSamples(traceData, f.BinaryHeader.DataSampleFormatCode, byteOrder)
}

func ibm32ToFloat32(raw uint32) float32 {
	if raw == 0 {
		return 0
	}

	sign := (raw >> 31) & 0x1
	exponent := int((raw>>24)&0x7F) - 64
	fraction := raw & 0x00FFFFFF

	mantissa := float64(fraction) / float64(1<<24)
	value := mantissa * math.Pow(16, float64(exponent))
	if sign == 1 {
		value = -value
	}

	return float32(value)
}

// ReadTraceFromIndex reads one trace referenced by a catalog entry.
func (f *File) ReadTraceFromIndex(entry TraceIndexEntry) (TraceHeader, []byte, error) {
	return f.ReadTraceAtOffset(entry.HeaderOffset, entry.SampleCount)
}

// ReadDecodedTraceFromIndex reads one trace and decodes sample bytes to float32 values.
func (f *File) ReadDecodedTraceFromIndex(entry TraceIndexEntry) (TraceHeader, []float32, error) {
	th, traceData, err := f.ReadTraceFromIndex(entry)
	if err != nil {
		return TraceHeader{}, nil, err
	}

	samples, err := f.DecodeTraceData(traceData)
	if err != nil {
		return TraceHeader{}, nil, err
	}

	return th, samples, nil
}

// ReadDecodedTraceAtOffset reads one trace at a header offset and decodes to float32 values.
func (f *File) ReadDecodedTraceAtOffset(headerOffset int64, sampleCount uint16) (TraceHeader, []float32, error) {
	th, traceData, err := f.ReadTraceAtOffset(headerOffset, sampleCount)
	if err != nil {
		return TraceHeader{}, nil, err
	}

	samples, err := f.DecodeTraceData(traceData)
	if err != nil {
		return TraceHeader{}, nil, err
	}

	return th, samples, nil
}

// ReadEnsembleSamples reads and decodes traces, returning a rectangular 2D sample set.
// Traces shorter than the longest trace are padded with zeros.
// If maxTotalImageSamples > 0 and the estimated image sample count exceeds it,
// a TooManySamplesError is returned before reading trace data.
func (f *File) ReadEnsembleSamples(entries []TraceIndexEntry, maxTotalImageSamples int64) (EnsembleSamples, error) {
	if f == nil {
		return EnsembleSamples{}, fmt.Errorf("nil segy file")
	}
	if len(entries) == 0 {
		return EnsembleSamples{}, nil
	}

	maxSamplesPerTrace := 0
	for _, entry := range entries {
		s := int(entry.SampleCount)
		if s == 0 {
			s = int(f.BinaryHeader.SamplesPerTrace)
		}
		if s > maxSamplesPerTrace {
			maxSamplesPerTrace = s
		}
	}
	if maxSamplesPerTrace <= 0 {
		return EnsembleSamples{}, fmt.Errorf("cannot determine sample count for ensemble")
	}

	totalImageSamples := int64(len(entries)) * int64(maxSamplesPerTrace)
	if maxTotalImageSamples > 0 && totalImageSamples > maxTotalImageSamples {
		return EnsembleSamples{}, TooManySamplesError{Requested: totalImageSamples, Threshold: maxTotalImageSamples}
	}

	traces := make([][]float32, len(entries))
	for i, entry := range entries {
		_, samples, err := f.ReadDecodedTraceFromIndex(entry)
		if err != nil {
			return EnsembleSamples{}, fmt.Errorf("read/decode trace %d: %w", i+1, err)
		}

		if len(samples) < maxSamplesPerTrace {
			padded := make([]float32, maxSamplesPerTrace)
			copy(padded, samples)
			traces[i] = padded
			continue
		}

		if len(samples) > maxSamplesPerTrace {
			samples = samples[:maxSamplesPerTrace]
		}
		traces[i] = samples
	}

	return EnsembleSamples{
		Traces:            traces,
		TraceCount:        len(traces),
		SamplesPerTrace:   maxSamplesPerTrace,
		TotalImageSamples: totalImageSamples,
	}, nil
}

// RenderEnsembleImage converts 2D trace samples to an image.Paletted using the provided options.
func RenderEnsembleImage(samples EnsembleSamples, opts EnsembleImageOptions) (*image.Paletted, EnsembleImageStats, error) {
	if len(opts.Palette) != 256 {
		return nil, EnsembleImageStats{}, fmt.Errorf("palette length must be 256, got %d", len(opts.Palette))
	}
	if samples.TraceCount == 0 || samples.SamplesPerTrace == 0 {
		return nil, EnsembleImageStats{}, fmt.Errorf("empty ensemble samples")
	}

	if !opts.ZeroCentered {
		opts.ZeroIndex = 127
	}

	img := image.NewPaletted(image.Rect(0, 0, samples.TraceCount, samples.SamplesPerTrace), opts.Palette)
	workingTraces := applyGain(samples.Traces, opts.Gain)

	stats := EnsembleImageStats{
		ZeroIndex: opts.ZeroIndex,
		Width:     samples.TraceCount,
		Height:    samples.SamplesPerTrace,
	}

	if opts.ZeroCentered {
		negClip := opts.NegativePercentileClip
		if negClip <= 0 {
			negClip = opts.PercentileClip
		}
		posClip := opts.PositivePercentileClip
		if posClip <= 0 {
			posClip = opts.PercentileClip
		}

		negatives, positives := splitSigns(workingTraces)
		lower := float32(0)
		upper := float32(0)
		if len(negatives) > 0 {
			lower = percentileValue(negatives, clampPercent(negClip))
		}
		if len(positives) > 0 {
			upper = percentileValue(positives, 100.0-clampPercent(posClip))
		}

		stats.LowerClip = lower
		stats.UpperClip = upper

		for x := 0; x < samples.TraceCount; x++ {
			trace := workingTraces[x]
			for y := 0; y < samples.SamplesPerTrace; y++ {
				v := trace[y]
				idx := mapZeroCentered(v, lower, upper, opts.ZeroIndex)
				img.SetColorIndex(x, y, idx)
			}
		}

		return img, stats, nil
	}

	flat := flattenFinite(workingTraces)
	if len(flat) == 0 {
		return nil, EnsembleImageStats{}, fmt.Errorf("no finite sample values available for rendering")
	}

	clip := clampPercent(opts.PercentileClip)
	lower := percentileValue(flat, clip)
	upper := percentileValue(flat, 100.0-clip)
	if lower > upper {
		lower, upper = upper, lower
	}

	stats.LowerClip = lower
	stats.UpperClip = upper

	for x := 0; x < samples.TraceCount; x++ {
		trace := workingTraces[x]
		for y := 0; y < samples.SamplesPerTrace; y++ {
			idx := mapLinear(trace[y], lower, upper)
			img.SetColorIndex(x, y, idx)
		}
	}

	return img, stats, nil
}

func applyGain(traces [][]float32, gain GainOptions) [][]float32 {
	switch gain.Mode {
	case GainModeTSquared:
		return applyTSquaredGain(traces, gain)
	case GainModeAGC:
		return applyAGC(traces, gain)
	default:
		return traces
	}
}

func applyTSquaredGain(traces [][]float32, gain GainOptions) [][]float32 {
	out := cloneTraces(traces)
	scale := gain.TSquaredScale
	if scale <= 0 {
		scale = 1
	}
	dt := gain.SampleIntervalSec

	for i := range out {
		trace := out[i]
		for j := range trace {
			t := float64(j)
			if dt > 0 {
				t *= dt
			}
			gainFactor := 1.0 + scale*t*t
			trace[j] *= float32(gainFactor)
		}
	}

	return out
}

func applyAGC(traces [][]float32, gain GainOptions) [][]float32 {
	out := cloneTraces(traces)
	window := gain.AGCWindowSamples
	if window <= 0 {
		window = 101
	}
	if window%2 == 0 {
		window++
	}
	half := window / 2
	floor := gain.AGCFloor
	if floor <= 0 {
		floor = 1e-6
	}

	for i := range out {
		trace := out[i]
		n := len(trace)
		if n == 0 {
			continue
		}

		prefix := make([]float64, n+1)
		for j := 0; j < n; j++ {
			v := float64(trace[j])
			prefix[j+1] = prefix[j] + v*v
		}

		for j := 0; j < n; j++ {
			start := j - half
			if start < 0 {
				start = 0
			}
			end := j + half + 1
			if end > n {
				end = n
			}
			count := end - start
			if count <= 0 {
				continue
			}

			sumSq := prefix[end] - prefix[start]
			rms := math.Sqrt(sumSq / float64(count))
			if rms < floor {
				rms = floor
			}
			trace[j] = float32(float64(trace[j]) / rms)
		}
	}

	return out
}

func cloneTraces(traces [][]float32) [][]float32 {
	out := make([][]float32, len(traces))
	for i := range traces {
		if traces[i] == nil {
			continue
		}
		copied := make([]float32, len(traces[i]))
		copy(copied, traces[i])
		out[i] = copied
	}
	return out
}

// RenderEnsembleFromEntries reads, decodes, and renders selected traces as a paletted image.
// If maxTotalImageSamples <= 0, DefaultMaxImageSamples is used.
func (f *File) RenderEnsembleFromEntries(entries []TraceIndexEntry, maxTotalImageSamples int64, opts EnsembleImageOptions) (*image.Paletted, EnsembleImageStats, error) {
	if f == nil {
		return nil, EnsembleImageStats{}, fmt.Errorf("nil segy file")
	}
	if maxTotalImageSamples <= 0 {
		maxTotalImageSamples = DefaultMaxImageSamples
	}

	ensemble, err := f.ReadEnsembleSamples(entries, maxTotalImageSamples)
	if err != nil {
		return nil, EnsembleImageStats{}, err
	}

	return RenderEnsembleImage(ensemble, opts)
}

// QueryAndRenderEnsemble finds traces using predicates, optionally sorts them,
// then reads, decodes, and renders a paletted ensemble image.
// If maxTotalImageSamples <= 0, DefaultMaxImageSamples is used.
func (f *File) QueryAndRenderEnsemble(predicates []TraceQueryPredicate, sortKeys []TraceSortKey, maxTotalImageSamples int64, opts EnsembleImageOptions) (*image.Paletted, EnsembleImageStats, error) {
	if f == nil {
		return nil, EnsembleImageStats{}, fmt.Errorf("nil segy file")
	}
	if f.TraceCatalog == nil {
		if _, err := f.BuildTraceCatalog(); err != nil {
			return nil, EnsembleImageStats{}, err
		}
	}

	var (
		entries []TraceIndexEntry
		err     error
	)
	if len(sortKeys) > 0 {
		entries, err = f.TraceCatalog.FindWhereSorted(predicates, sortKeys...)
	} else {
		entries, err = f.TraceCatalog.FindWhere(predicates...)
	}
	if err != nil {
		return nil, EnsembleImageStats{}, err
	}

	return f.RenderEnsembleFromEntries(entries, maxTotalImageSamples, opts)
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 50 {
		return 50
	}
	return v
}

func flattenFinite(traces [][]float32) []float32 {
	out := make([]float32, 0)
	for _, trace := range traces {
		for _, v := range trace {
			if !isFinite(v) {
				continue
			}
			out = append(out, v)
		}
	}
	return out
}

func splitSigns(traces [][]float32) ([]float32, []float32) {
	negatives := make([]float32, 0)
	positives := make([]float32, 0)
	for _, trace := range traces {
		for _, v := range trace {
			if !isFinite(v) {
				continue
			}
			if v < 0 {
				negatives = append(negatives, v)
			} else if v > 0 {
				positives = append(positives, v)
			}
		}
	}
	return negatives, positives
}

func percentileValue(values []float32, p float64) float32 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		p = 0
	}
	if p >= 100 {
		p = 100
	}

	buf := make([]float32, len(values))
	copy(buf, values)
	sort.Slice(buf, func(i, j int) bool { return buf[i] < buf[j] })

	if len(buf) == 1 {
		return buf[0]
	}

	pos := (p / 100.0) * float64(len(buf)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return buf[lo]
	}
	w := float32(pos - float64(lo))
	return buf[lo]*(1.0-w) + buf[hi]*w
}

func mapLinear(v, lower, upper float32) uint8 {
	if !isFinite(v) {
		return 0
	}
	if upper <= lower {
		if v <= lower {
			return 0
		}
		return 255
	}
	t := (v - lower) / (upper - lower)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return uint8(math.Round(float64(t * 255)))
}

func mapZeroCentered(v, lower, upper float32, zeroIndex uint8) uint8 {
	if !isFinite(v) {
		return zeroIndex
	}
	zi := float32(zeroIndex)

	if v <= 0 {
		if lower >= 0 {
			return zeroIndex
		}
		t := (v - lower) / (0 - lower)
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		idx := t * zi
		return uint8(math.Round(float64(idx)))
	}

	if upper <= 0 {
		return zeroIndex
	}
	t := v / upper
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	idx := zi + t*(255.0-zi)
	return uint8(math.Round(float64(idx)))
}

func isFinite(v float32) bool {
	f := float64(v)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
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

// Asc returns an ascending sort key for a header field.
func Asc(field TraceHeaderField) TraceSortKey {
	return TraceSortKey{Field: field, Order: TraceSortAscending}
}

// Desc returns a descending sort key for a header field.
func Desc(field TraceHeaderField) TraceSortKey {
	return TraceSortKey{Field: field, Order: TraceSortDescending}
}

// SortTraceIndexEntries sorts entries in place using the provided keys.
// If no keys are provided, entries are sorted by TraceNumber in file order.
func SortTraceIndexEntries(entries []TraceIndexEntry, keys ...TraceSortKey) error {
	for _, key := range keys {
		if !isIndexedTraceHeaderField(key.Field) {
			return fmt.Errorf("unsupported sort field %q", key.Field)
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]

		for _, key := range keys {
			lv := traceHeaderFieldValue(key.Field, left.Header)
			rv := traceHeaderFieldValue(key.Field, right.Header)
			if lv == rv {
				continue
			}
			if key.Order == TraceSortDescending {
				return lv > rv
			}
			return lv < rv
		}

		return left.TraceNumber < right.TraceNumber
	})

	return nil
}

func isIndexedTraceHeaderField(field TraceHeaderField) bool {
	for _, indexed := range indexedTraceHeaderFields() {
		if indexed == field {
			return true
		}
	}
	return false
}

// GrayscalePalette256 returns a 256-color grayscale palette from black to white.
func GrayscalePalette256() color.Palette {
	p := make(color.Palette, 256)
	for i := 0; i < 256; i++ {
		v := uint8(i)
		p[i] = color.RGBA{R: v, G: v, B: v, A: 255}
	}
	return p
}

// BlueWhiteRedPalette256 returns a 256-color diverging palette from blue to white to red.
func BlueWhiteRedPalette256() color.Palette {
	p := make(color.Palette, 256)
	for i := 0; i < 256; i++ {
		t := float64(i) / 255.0
		if t <= 0.5 {
			u := t / 0.5
			p[i] = lerpRGBA(color.RGBA{R: 0, G: 0, B: 255, A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}, u)
			continue
		}

		u := (t - 0.5) / 0.5
		p[i] = lerpRGBA(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{R: 255, G: 0, B: 0, A: 255}, u)
	}
	return p
}

// InvertPalette returns a reversed copy of the input palette.
func InvertPalette(p color.Palette) color.Palette {
	if len(p) == 0 {
		return color.Palette{}
	}
	out := make(color.Palette, len(p))
	for i := range p {
		out[i] = p[len(p)-1-i]
	}
	return out
}

// MustPalette256 normalizes a palette to length 256.
// It panics when given an empty palette.
func MustPalette256(p color.Palette) color.Palette {
	if len(p) == 0 {
		panic("segy: palette must contain at least one color")
	}
	if len(p) == 256 {
		out := make(color.Palette, 256)
		copy(out, p)
		return out
	}

	out := make(color.Palette, 256)
	if len(p) == 1 {
		for i := 0; i < 256; i++ {
			out[i] = p[0]
		}
		return out
	}

	last := len(p) - 1
	for i := 0; i < 256; i++ {
		pos := float64(i) * float64(last) / 255.0
		idx := int(math.Round(pos))
		if idx < 0 {
			idx = 0
		}
		if idx > last {
			idx = last
		}
		out[i] = p[idx]
	}

	return out
}

func lerpRGBA(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t))
	}
	return color.RGBA{
		R: lerp(a.R, b.R),
		G: lerp(a.G, b.G),
		B: lerp(a.B, b.B),
		A: lerp(a.A, b.A),
	}
}
