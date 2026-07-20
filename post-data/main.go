package main

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "post-data",
	Short: "Post one or more objects to MinIO",
	Long: `Post one or more objects to MinIO with a custom timestamp and optional metadata.

The object (-o) may include the bucket as a prefix:

  -o mybucket/path/to/key

When using --alias the object may also carry the alias prefix:

  -o myalias/mybucket/path/to/key`,
	RunE: runPost,
}

var (
	flagAlias       string
	flagEndpoint    string
	flagAccessKey   string
	flagSecretKey   string
	flagObject      string
	flagData        string
	flagTimestamp   string
	flagMetadata    []string
	flagInsecure    bool
	flagContentType string
	flagVersioning  bool
	flagCount       int
	flagRandomData  bool
	flagMaxSize     string
	flagNamePrefix  string
)

func init() {
	rootCmd.Flags().StringVar(&flagAlias, "alias", "", "mc alias name (reads endpoint/credentials from ~/.mc/config.json)")
	rootCmd.Flags().StringVarP(&flagEndpoint, "endpoint", "e", "", "MinIO endpoint (host:port or full URL)")
	rootCmd.Flags().StringVarP(&flagAccessKey, "access-key", "a", "", "MinIO access key")
	rootCmd.Flags().StringVarP(&flagSecretKey, "secret-key", "s", "", "MinIO secret key")
	rootCmd.Flags().StringVarP(&flagObject, "object", "o", "", "bucket/object key, optionally prefixed with alias/ (required)")
	rootCmd.Flags().StringVarP(&flagData, "data", "d", "", "Data content (text string)")
	rootCmd.Flags().StringVarP(&flagTimestamp, "timestamp", "t", "now",
		"Timestamp: 'now', '-Nd' (N days ago), '+Nd' or 'Nd' (N days from now), or RFC3339 (e.g. 2024-01-15T10:00:00Z)")
	rootCmd.Flags().StringArrayVarP(&flagMetadata, "meta", "m", nil, "Metadata as key=value (repeatable, e.g. -m author=john -m env=test)")
	rootCmd.Flags().BoolVar(&flagInsecure, "insecure", false, "Force HTTP (overrides alias URL scheme)")
	rootCmd.Flags().StringVar(&flagContentType, "content-type", "text/plain", "Content-Type of the object")
	rootCmd.Flags().BoolVar(&flagVersioning, "versioning", true, "Enable versioning when auto-creating a bucket")
	rootCmd.Flags().IntVarP(&flagCount, "count", "n", 1, "Number of objects to post (>1 appends a sequential suffix to the object key)")
	rootCmd.Flags().BoolVar(&flagRandomData, "random-data", false, "Fill each object with random bytes instead of using --data")
	rootCmd.Flags().StringVar(&flagMaxSize, "max-size", "1KiB", "Maximum size for random data, e.g. 512, 4KiB, 2MiB (used with --random-data)")
	rootCmd.Flags().StringVar(&flagNamePrefix, "name-prefix", "", "Prefix prepended to each generated object name, e.g. 'log-' → log-001, log-002")

	rootCmd.MarkFlagRequired("object")
}

// mcAlias represents one entry in ~/.mc/config.json
type mcAlias struct {
	URL       string `json:"url"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type mcConfig struct {
	Aliases map[string]mcAlias `json:"aliases"`
}

func loadMCAlias(name string) (mcAlias, error) {
	cfgPath := filepath.Join(os.Getenv("HOME"), ".mc", "config.json")
	if d := os.Getenv("MC_CONFIG_DIR"); d != "" {
		cfgPath = filepath.Join(d, "config.json")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return mcAlias{}, fmt.Errorf("reading mc config %s: %w", cfgPath, err)
	}

	var cfg mcConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return mcAlias{}, fmt.Errorf("parsing mc config: %w", err)
	}

	a, ok := cfg.Aliases[name]
	if !ok {
		names := make([]string, 0, len(cfg.Aliases))
		for k := range cfg.Aliases {
			names = append(names, k)
		}
		return mcAlias{}, fmt.Errorf("alias %q not found in mc config (available: %s)", name, strings.Join(names, ", "))
	}
	return a, nil
}

// parseBucketObject splits -o value into bucket and object key.
// Accepted forms:
//
//	bucket/key
//	alias/bucket/key  (alias prefix stripped when --alias matches)
func parseBucketObject(raw, aliasName string) (bucket, object string, err error) {
	s := raw
	if aliasName != "" && strings.HasPrefix(s, aliasName+"/") {
		s = s[len(aliasName)+1:]
	}
	i := strings.IndexByte(s, '/')
	if i <= 0 {
		return "", "", fmt.Errorf("-o %q must be in bucket/object format", raw)
	}
	return s[:i], s[i+1:], nil
}

func parseTimestamp(ts string) (time.Time, error) {
	ts = strings.TrimSpace(ts)

	if ts == "" || ts == "now" {
		return time.Now(), nil
	}

	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t, nil
	}

	sign := 1
	raw := ts
	if strings.HasPrefix(ts, "-") {
		sign = -1
		raw = ts[1:]
	} else if strings.HasPrefix(ts, "+") {
		raw = ts[1:]
	}

	if strings.HasSuffix(raw, "d") {
		numStr := strings.TrimSuffix(raw, "d")
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid day offset %q: %w", ts, err)
		}
		return time.Now().Add(time.Duration(sign*n) * 24 * time.Hour), nil
	}

	return time.Time{}, fmt.Errorf("unrecognized timestamp format %q (use 'now', '-Nd', '+Nd', 'Nd', or RFC3339)", ts)
}

// randomData returns a random byte slice of random length in [1, maxSize].
func randomData(maxSize int64) []byte {
	if maxSize < 1 {
		maxSize = 1
	}
	size := rand.Int63n(maxSize) + 1
	buf := make([]byte, size)
	_, _ = crand.Read(buf)
	return buf
}

// sequentialObjectKey appends a zero-padded sequence number to prefix.
// If prefix already ends with '/' the suffix is appended directly,
// otherwise a '/' separator is inserted.
// width is the total number of digits (derived from the total count).
func sequentialObjectKey(prefix, namePrefix string, seq, width int) string {
	suffix := namePrefix + fmt.Sprintf("%0*d", width, seq)
	if prefix == "" || strings.HasSuffix(prefix, "/") {
		return prefix + suffix
	}
	return prefix + "/" + suffix
}

func parseMetadata(pairs []string) (map[string]string, error) {
	meta := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("metadata %q is not in key=value format", pair)
		}
		meta[k] = v
	}
	return meta, nil
}

func runPost(cmd *cobra.Command, args []string) error {
	endpoint := flagEndpoint
	accessKey := flagAccessKey
	secretKey := flagSecretKey
	useSSL := false

	if flagAlias != "" {
		a, err := loadMCAlias(flagAlias)
		if err != nil {
			return err
		}
		u := a.URL
		if strings.HasPrefix(u, "https://") {
			useSSL = true
			u = strings.TrimPrefix(u, "https://")
		} else {
			u = strings.TrimPrefix(u, "http://")
		}
		if endpoint == "" {
			endpoint = u
		}
		if accessKey == "" {
			accessKey = a.AccessKey
		}
		if secretKey == "" {
			secretKey = a.SecretKey
		}
		fmt.Printf("using alias %q  endpoint=%s\n", flagAlias, endpoint)
	}

	if endpoint == "" {
		endpoint = "localhost:9000"
	}

	if flagInsecure {
		useSSL = false
	}

	bucket, object, err := parseBucketObject(flagObject, flagAlias)
	if err != nil {
		return err
	}

	liveTimestamp := !cmd.Flags().Changed("timestamp")

	ts, err := parseTimestamp(flagTimestamp)
	if err != nil {
		return err
	}

	meta, err := parseMetadata(flagMetadata)
	if err != nil {
		return err
	}

	data := flagData
	if data == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			buf, err := os.ReadFile("/dev/stdin")
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			data = string(buf)
		}
	}

	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return fmt.Errorf("creating minio client: %w", err)
	}

	ctx := context.Background()

	exists, err := mc.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		if err := mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("creating bucket %q: %w", bucket, err)
		}
		fmt.Printf("created bucket %q\n", bucket)
		if flagVersioning {
			if err := mc.EnableVersioning(ctx, bucket); err != nil {
				return fmt.Errorf("enabling versioning on %q: %w", bucket, err)
			}
			fmt.Printf("versioning enabled on %q\n", bucket)
		}
	}

	count := flagCount
	if count < 1 {
		count = 1
	}
	width := len(fmt.Sprintf("%d", count))

	var maxSizeBytes int64
	if flagRandomData {
		parsed, err := humanize.ParseBytes(flagMaxSize)
		if err != nil {
			return fmt.Errorf("invalid --max-size %q: %w", flagMaxSize, err)
		}
		maxSizeBytes = int64(parsed)
	}

	for i := 0; i < count; i++ {
		objectKey := object
		if count > 1 {
			objectKey = sequentialObjectKey(object, flagNamePrefix, i+1, width)
		}

		effectiveTS := ts
		if liveTimestamp {
			effectiveTS = time.Now()
		}

		userMeta := make(map[string]string, len(meta)+1)
		for k, v := range meta {
			userMeta[k] = v
		}
		userMeta["x-posted-timestamp"] = effectiveTS.UTC().Format(time.RFC3339)

		opts := minio.PutObjectOptions{
			ContentType:  flagContentType,
			UserMetadata: userMeta,
			Internal: minio.AdvancedPutOptions{
				SourceMTime: effectiveTS,
			},
		}

		var payload []byte
		if flagRandomData {
			payload = randomData(maxSizeBytes)
		} else {
			payload = []byte(data)
		}

		reader := strings.NewReader(string(payload))
		size := int64(len(payload))

		info, err := mc.PutObject(ctx, bucket, objectKey, reader, size, opts)
		if err != nil {
			return fmt.Errorf("uploading object %d/%d: %w", i+1, count, err)
		}

		fmt.Printf("uploaded: bucket=%s object=%s size=%d etag=%s timestamp=%s\n",
			info.Bucket, info.Key, info.Size, info.ETag, effectiveTS.UTC().Format(time.RFC3339))

		if i == 0 && len(meta) > 0 {
			fmt.Println("metadata:")
			for k, v := range meta {
				fmt.Printf("  %s = %s\n", k, v)
			}
		}
	}
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
