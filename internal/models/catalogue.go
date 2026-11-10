// Package models knows which Whisper and voice-activity-detection model files
// slonik-ears can use, and fetches them with their checksums verified.
package models

import (
	"fmt"
	"slices"
	"strings"
)

// Model describes one downloadable model file.
type Model struct {
	// Name is the file name without the ggml- prefix and .bin suffix, for
	// example large-v3 or small.en-q5_1.
	Name string
	// File is the name the model is stored under on disk.
	File string
	// URL is the pinned download address.
	URL string
	// SHA256 is the lower-case hex digest of the complete file.
	SHA256 string
	// Size is the length of the file in bytes.
	Size int64
}

// vadName is the Silero voice-activity-detection model that ships by default.
const vadName = "silero-v5.1.2"

// Names returns every model name in the catalogue, sorted.
func Names() []string {
	names := make([]string, 0, len(catalogue))
	for _, m := range catalogue {
		names = append(names, m.Name)
	}
	slices.Sort(names)
	return names
}

// Lookup returns the named model. An unknown name produces an error that lists
// every name that would have been accepted.
func Lookup(name string) (Model, error) {
	for _, m := range catalogue {
		if m.Name == name {
			return m, nil
		}
	}
	all := append(Names(), BundleNames()...)
	return Model{}, fmt.Errorf("unknown model %q; available models: %s", name, strings.Join(all, ", "))
}

// Bundle is a model made of several files that are stored together in a
// directory named after it, as the sherpa-onnx Parakeet models are.
type Bundle struct {
	// Name is the bundle name, and the directory its files are stored in.
	Name string
	// Files are the pieces of the bundle. Each file's Name is the bundle name,
	// a slash and the file name.
	Files []Model
}

// BundleNames returns every bundle name in the catalogue, sorted.
func BundleNames() []string {
	names := make([]string, 0, len(bundles))
	for _, b := range bundles {
		names = append(names, b.Name)
	}
	slices.Sort(names)
	return names
}

// LookupBundle returns the named bundle, and false if there is none.
func LookupBundle(name string) (Bundle, bool) {
	for _, b := range bundles {
		if b.Name == name {
			return b, true
		}
	}
	return Bundle{}, false
}

// parakeetBundle builds a bundle from the sherpa-onnx repository at the pinned
// commit named in base, with the four files each Parakeet model needs.
func parakeetBundle(name, base string, files [4]Model) Bundle {
	b := Bundle{Name: name}
	for _, f := range files {
		f.Name = name + "/" + f.File
		f.URL = base + f.File
		b.Files = append(b.Files, f)
	}
	return b
}

// The Parakeet bundles were taken from the Hugging Face tree API at the pinned
// commits named in their URLs, in the same way as the table further down:
// SHA256 is each entry's lfs.oid and Size its lfs.size.
var bundles = []Bundle{
	parakeetBundle(
		"parakeet-tdt-0.6b-v2-int8",
		"https://huggingface.co/csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v2-int8/resolve/84b478d541cdbf0c296a7ab5f1e20a6f8f998114/",
		[4]Model{
			{File: "encoder.int8.onnx", SHA256: "92a10bd9c23936ea0566c29f963bc57c2a5df4261b6a8921c39abea66557ca8b", Size: 652282309},
			{File: "decoder.int8.onnx", SHA256: "4d097bae3bac5c96018d5293ba39f1b657296102b142affda40378f38b020f41", Size: 7257753},
			{File: "joiner.int8.onnx", SHA256: "77a29f3e5ac7d37074e0af465391b80f314015d5b97f0f90c6c7767f1d56f428", Size: 1739080},
			{File: "tokens.txt", SHA256: "ec182b70dd42113aff6c5372c75cac58c952443eb22322f57bbd7f53977d497d", Size: 9384},
		}),
	parakeetBundle(
		"parakeet-tdt-0.6b-v3-int8",
		"https://huggingface.co/csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8/resolve/ed5d745623472f97cfc2e6c2916ee248798fc5a8/",
		[4]Model{
			{File: "encoder.int8.onnx", SHA256: "1afbe69c1ffdabee94755f42e4daf1d93549131a3da11356bda84fcef0f2c479", Size: 652282294},
			{File: "decoder.int8.onnx", SHA256: "0222a86147299d4dee90edf95b6d13827edb42ad8b7c9cf0b2582f030937c0b5", Size: 11845274},
			{File: "joiner.int8.onnx", SHA256: "cbd02f3696e9944970111fb439d9c3fd365a80374565f13f2b9c7007d253fc98", Size: 6355277},
			{File: "tokens.txt", SHA256: "d58544679ea4bc6ac563d1f545eb7d474bd6cfa467f0a6e2c1dc1c7d37e3c35d", Size: 93939},
		}),
}

// VAD returns the Silero voice-activity-detection model.
func VAD() Model {
	m, err := Lookup(vadName)
	if err != nil {
		// The catalogue is a compiled-in table, so this is a programming error.
		panic(err)
	}
	return m
}

// The table below was generated, not written by hand, from the Hugging Face
// tree API at two pinned commits:
//
//	curl -s https://huggingface.co/api/models/ggerganov/whisper.cpp/tree/5359861c739e955e79d9a303bcbc70fb988958b1
//	curl -s https://huggingface.co/api/models/ggml-org/whisper-vad/tree/9ffd54a1e1ee413ddf265af9913beaf518d1639b
//
// Every ggml-*.bin is listed except the large-v1 and large-v2 builds. SHA256
// is each entry's lfs.oid and Size its lfs.size. URLs name the commit rather
// than main, so a download cannot change underneath the checksum.
var catalogue = []Model{
	{
		Name:   "base",
		File:   "ggml-base.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base.bin",
		SHA256: "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe",
		Size:   147951465,
	},
	{
		Name:   "base-q5_1",
		File:   "ggml-base-q5_1.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base-q5_1.bin",
		SHA256: "422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898",
		Size:   59707625,
	},
	{
		Name:   "base-q8_0",
		File:   "ggml-base-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base-q8_0.bin",
		SHA256: "c577b9a86e7e048a0b7eada054f4dd79a56bbfa911fbdacf900ac5b567cbb7d9",
		Size:   81768585,
	},
	{
		Name:   "base.en",
		File:   "ggml-base.en.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base.en.bin",
		SHA256: "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002",
		Size:   147964211,
	},
	{
		Name:   "base.en-q5_1",
		File:   "ggml-base.en-q5_1.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base.en-q5_1.bin",
		SHA256: "4baf70dd0d7c4247ba2b81fafd9c01005ac77c2f9ef064e00dcf195d0e2fdd2f",
		Size:   59721011,
	},
	{
		Name:   "base.en-q8_0",
		File:   "ggml-base.en-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base.en-q8_0.bin",
		SHA256: "a4d4a0768075e13cfd7e19df3ae2dbc4a68d37d36a7dad45e8410c9a34f8c87e",
		Size:   81781811,
	},
	{
		Name:   "large-v3",
		File:   "ggml-large-v3.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3.bin",
		SHA256: "64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2",
		Size:   3095033483,
	},
	{
		Name:   "large-v3-q5_0",
		File:   "ggml-large-v3-q5_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3-q5_0.bin",
		SHA256: "d75795ecff3f83b5faa89d1900604ad8c780abd5739fae406de19f23ecd98ad1",
		Size:   1081140203,
	},
	{
		Name:   "large-v3-turbo",
		File:   "ggml-large-v3-turbo.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3-turbo.bin",
		SHA256: "1fc70f774d38eb169993ac391eea357ef47c88757ef72ee5943879b7e8e2bc69",
		Size:   1624555275,
	},
	{
		Name:   "large-v3-turbo-q5_0",
		File:   "ggml-large-v3-turbo-q5_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3-turbo-q5_0.bin",
		SHA256: "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2",
		Size:   574041195,
	},
	{
		Name:   "large-v3-turbo-q8_0",
		File:   "ggml-large-v3-turbo-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3-turbo-q8_0.bin",
		SHA256: "317eb69c11673c9de1e1f0d459b253999804ec71ac4c23c17ecf5fbe24e259a1",
		Size:   874188075,
	},
	{
		Name:   "medium",
		File:   "ggml-medium.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-medium.bin",
		SHA256: "6c14d5adee5f86394037b4e4e8b59f1673b6cee10e3cf0b11bbdbee79c156208",
		Size:   1533763059,
	},
	{
		Name:   "medium-q5_0",
		File:   "ggml-medium-q5_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-medium-q5_0.bin",
		SHA256: "19fea4b380c3a618ec4723c3eef2eb785ffba0d0538cf43f8f235e7b3b34220f",
		Size:   539212467,
	},
	{
		Name:   "medium-q8_0",
		File:   "ggml-medium-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-medium-q8_0.bin",
		SHA256: "42a1ffcbe4167d224232443396968db4d02d4e8e87e213d3ee2e03095dea6502",
		Size:   823369779,
	},
	{
		Name:   "medium.en",
		File:   "ggml-medium.en.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-medium.en.bin",
		SHA256: "cc37e93478338ec7700281a7ac30a10128929eb8f427dda2e865faa8f6da4356",
		Size:   1533774781,
	},
	{
		Name:   "medium.en-q5_0",
		File:   "ggml-medium.en-q5_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-medium.en-q5_0.bin",
		SHA256: "76733e26ad8fe1c7a5bf7531a9d41917b2adc0f20f2e4f5531688a8c6cd88eb0",
		Size:   539225533,
	},
	{
		Name:   "medium.en-q8_0",
		File:   "ggml-medium.en-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-medium.en-q8_0.bin",
		SHA256: "43fa2cd084de5a04399a896a9a7a786064e221365c01700cea4666005218f11c",
		Size:   823382461,
	},
	{
		Name:   "silero-v5.1.2",
		File:   "ggml-silero-v5.1.2.bin",
		URL:    "https://huggingface.co/ggml-org/whisper-vad/resolve/9ffd54a1e1ee413ddf265af9913beaf518d1639b/ggml-silero-v5.1.2.bin",
		SHA256: "29940d98d42b91fbd05ce489f3ecf7c72f0a42f027e4875919a28fb4c04ea2cf",
		Size:   885098,
	},
	{
		Name:   "silero-v6.2.0",
		File:   "ggml-silero-v6.2.0.bin",
		URL:    "https://huggingface.co/ggml-org/whisper-vad/resolve/9ffd54a1e1ee413ddf265af9913beaf518d1639b/ggml-silero-v6.2.0.bin",
		SHA256: "2aa269b785eeb53a82983a20501ddf7c1d9c48e33ab63a41391ac6c9f7fb6987",
		Size:   885098,
	},
	{
		Name:   "small",
		File:   "ggml-small.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-small.bin",
		SHA256: "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b",
		Size:   487601967,
	},
	{
		Name:   "small-q5_1",
		File:   "ggml-small-q5_1.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-small-q5_1.bin",
		SHA256: "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb",
		Size:   190085487,
	},
	{
		Name:   "small-q8_0",
		File:   "ggml-small-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-small-q8_0.bin",
		SHA256: "49c8fb02b65e6049d5fa6c04f81f53b867b5ec9540406812c643f177317f779f",
		Size:   264464607,
	},
	{
		Name:   "small.en",
		File:   "ggml-small.en.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-small.en.bin",
		SHA256: "c6138d6d58ecc8322097e0f987c32f1be8bb0a18532a3f88f734d1bbf9c41e5d",
		Size:   487614201,
	},
	{
		Name:   "small.en-q5_1",
		File:   "ggml-small.en-q5_1.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-small.en-q5_1.bin",
		SHA256: "bfdff4894dcb76bbf647d56263ea2a96645423f1669176f4844a1bf8e478ad30",
		Size:   190098681,
	},
	{
		Name:   "small.en-q8_0",
		File:   "ggml-small.en-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-small.en-q8_0.bin",
		SHA256: "67a179f608ea6114bd3fdb9060e762b588a3fb3bd00c4387971be4d177958067",
		Size:   264477561,
	},
	{
		Name:   "tiny",
		File:   "ggml-tiny.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny.bin",
		SHA256: "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21",
		Size:   77691713,
	},
	{
		Name:   "tiny-q5_1",
		File:   "ggml-tiny-q5_1.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny-q5_1.bin",
		SHA256: "818710568da3ca15689e31a743197b520007872ff9576237bda97bd1b469c3d7",
		Size:   32152673,
	},
	{
		Name:   "tiny-q8_0",
		File:   "ggml-tiny-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny-q8_0.bin",
		SHA256: "c2085835d3f50733e2ff6e4b41ae8a2b8d8110461e18821b09a15c40c42d1cca",
		Size:   43537433,
	},
	{
		Name:   "tiny.en",
		File:   "ggml-tiny.en.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny.en.bin",
		SHA256: "921e4cf8686fdd993dcd081a5da5b6c365bfde1162e72b08d75ac75289920b1f",
		Size:   77704715,
	},
	{
		Name:   "tiny.en-q5_1",
		File:   "ggml-tiny.en-q5_1.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny.en-q5_1.bin",
		SHA256: "c77c5766f1cef09b6b7d47f21b546cbddd4157886b3b5d6d4f709e91e66c7c2b",
		Size:   32166155,
	},
	{
		Name:   "tiny.en-q8_0",
		File:   "ggml-tiny.en-q8_0.bin",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny.en-q8_0.bin",
		SHA256: "5bc2b3860aa151a4c6e7bb095e1fcce7cf12c7b020ca08dcec0c6d018bb7dd94",
		Size:   43550795,
	},
}
