package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/spf13/cobra"
)

type Config struct {
	Endpoint       string
	AccessKey      string
	SecretKey      string
	Buckets        string
	UseSSL         bool
	MCAlias        string
	Duration       time.Duration
	OperationDelay time.Duration
	ObjectPrefix   string
}

type MinioClient struct {
	client *minio.Client
	config Config
	stats  *Stats
}

// parseBuckets parses comma-separated bucket names
func (m *MinioClient) parseBuckets() []string {
	if m.config.Buckets == "" {
		return []string{}
	}

	buckets := strings.Split(m.config.Buckets, ",")
	for i := range buckets {
		buckets[i] = strings.TrimSpace(buckets[i])
	}

	// Remove empty strings
	var result []string
	for _, bucket := range buckets {
		if bucket != "" {
			result = append(result, bucket)
		}
	}

	return result
}

// getRandomBucket returns a random bucket from the configured buckets
func (m *MinioClient) getRandomBucket() (string, error) {
	buckets := m.parseBuckets()
	if len(buckets) == 0 {
		return "", fmt.Errorf("no buckets configured")
	}

	if len(buckets) == 1 {
		return buckets[0], nil
	}

	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(buckets))))
	if err != nil {
		return "", fmt.Errorf("failed to generate random bucket selection: %v", err)
	}

	return buckets[index.Int64()], nil
}

type Stats struct {
	ReadOps         int64
	WriteOps        int64
	OverwriteOps    int64
	DeleteOps       int64
	PrefixDeleteOps int64
	MultipartOps    int64
	ErrorOps        int64
}

var (
	config  Config
	rootCmd = &cobra.Command{
		Use:   "generate-s3-data",
		Short: "A tool that generates S3 data by performing random operations",
		Long: `A tool that generates S3 data by sending random operations (read, write, overwrite, delete, prefix delete, multipart upload) 
to a MinIO server. Can be used for testing and audit purposes.`,
		Run: runClient,
	}
)

func init() {
	rootCmd.Flags().StringVarP(&config.Endpoint, "endpoint", "e", "localhost:9000", "MinIO server endpoint")
	rootCmd.Flags().StringVarP(&config.AccessKey, "access-key", "a", "", "MinIO access key")
	rootCmd.Flags().StringVarP(&config.SecretKey, "secret-key", "s", "", "MinIO secret key")
	rootCmd.Flags().StringVarP(&config.Buckets, "buckets", "b", "test-bucket", "MinIO bucket names (comma-separated)")
	rootCmd.Flags().BoolVar(&config.UseSSL, "ssl", false, "Use SSL connection")
	rootCmd.Flags().StringVar(&config.MCAlias, "alias", "", "Use MC alias instead of access/secret keys")
	rootCmd.Flags().DurationVarP(&config.Duration, "duration", "d", 0, "Duration to run (0 for infinite)")
	rootCmd.Flags().DurationVar(&config.OperationDelay, "delay", 1*time.Second, "Delay between operations")
	rootCmd.Flags().StringVarP(&config.ObjectPrefix, "prefix", "p", "test-object", "Object name prefix")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runClient(cmd *cobra.Command, args []string) {
	// Initialize MinIO client
	client, err := initializeMinioClient()
	if err != nil {
		log.Fatalf("Failed to initialize MinIO client: %v", err)
	}

	minioClient := &MinioClient{
		client: client,
		config: config,
		stats:  &Stats{},
	}

	// Ensure bucket exists
	if err := minioClient.ensureBucket(); err != nil {
		log.Fatalf("Failed to ensure bucket exists: %v", err)
	}

	fmt.Printf("Starting S3 data generator...\n")
	fmt.Printf("Endpoint: %s\n", config.Endpoint)
	fmt.Printf("Buckets: %s\n", config.Buckets)
	fmt.Printf("Duration: %v (0 = infinite)\n", config.Duration)
	fmt.Printf("Operation Delay: %v\n", config.OperationDelay)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("=" + strings.Repeat("=", 50))

	// Start operations
	ctx := context.Background()
	if config.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Duration)
		defer cancel()
	}

	// Start stats printer in background
	go minioClient.printStats(ctx)

	// Run operations
	minioClient.runOperations(ctx)

	// Print final stats
	fmt.Println("\nFinal Statistics:")
	minioClient.printFinalStats()
}

func initializeMinioClient() (*minio.Client, error) {
	var creds *credentials.Credentials

	if config.MCAlias != "" {
		// Try to use MC alias (read from ~/.mc/config.json)
		mcConfig, err := readMCConfig(config.MCAlias)
		if err != nil {
			return nil, fmt.Errorf("failed to read MC alias '%s': %v", config.MCAlias, err)
		}
		config.Endpoint = mcConfig.URL
		config.AccessKey = mcConfig.AccessKey
		config.SecretKey = mcConfig.SecretKey
		config.UseSSL = strings.HasPrefix(mcConfig.URL, "https://")

		// Remove protocol from endpoint
		config.Endpoint = strings.TrimPrefix(config.Endpoint, "http://")
		config.Endpoint = strings.TrimPrefix(config.Endpoint, "https://")
	}

	if config.AccessKey != "" && config.SecretKey != "" {
		creds = credentials.NewStaticV4(config.AccessKey, config.SecretKey, "")
	} else {
		return nil, fmt.Errorf("either provide access-key and secret-key, or use alias")
	}

	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}

	return client, nil
}

type MCConfig struct {
	URL       string `json:"url"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	API       string `json:"api"`
	Path      string `json:"path"`
}

type MCConfigFile struct {
	Version string               `json:"version"`
	Aliases map[string]*MCConfig `json:"aliases"`
}

func readMCConfig(alias string) (*MCConfig, error) {
	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %v", err)
	}

	// Path to MC config file
	mcConfigPath := filepath.Join(homeDir, ".mc", "config.json")

	// Check if config file exists
	if _, err := os.Stat(mcConfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("MC config file not found at %s. Run 'mc alias set %s <url> <access-key> <secret-key>' first", mcConfigPath, alias)
	}

	// Read the config file
	configData, err := os.ReadFile(mcConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MC config file: %v", err)
	}

	// Parse JSON
	var mcConfigFile MCConfigFile
	if err := json.Unmarshal(configData, &mcConfigFile); err != nil {
		return nil, fmt.Errorf("failed to parse MC config JSON: %v", err)
	}

	// Find the alias
	aliasConfig, exists := mcConfigFile.Aliases[alias]
	if !exists {
		return nil, fmt.Errorf("alias '%s' not found in MC config. Available aliases: %v", alias, getAvailableAliases(mcConfigFile.Aliases))
	}

	// Validate required fields
	if aliasConfig.URL == "" || aliasConfig.AccessKey == "" || aliasConfig.SecretKey == "" {
		return nil, fmt.Errorf("alias '%s' has incomplete configuration (missing URL, access key, or secret key)", alias)
	}

	return aliasConfig, nil
}

func getAvailableAliases(aliases map[string]*MCConfig) []string {
	var keys []string
	for k := range aliases {
		keys = append(keys, k)
	}
	return keys
}

func (m *MinioClient) ensureBucket() error {
	ctx := context.Background()
	buckets := m.parseBuckets()

	if len(buckets) == 0 {
		return fmt.Errorf("no buckets configured")
	}

	for _, bucket := range buckets {
		exists, err := m.client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("failed to check if bucket '%s' exists: %v", bucket, err)
		}

		if !exists {
			err = m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
			if err != nil {
				return fmt.Errorf("failed to create bucket '%s': %v", bucket, err)
			}
			fmt.Printf("Created bucket: %s\n", bucket)
		}
	}

	return nil
}

func (m *MinioClient) runOperations(ctx context.Context) {
	operations := []func() error{
		m.writeOperation,
		m.readOperation,
		m.overwriteOperation,
		m.deleteOperation,
		m.prefixDeleteOperation,
		m.multipartWriteOperation,
	}

	ticker := time.NewTicker(m.config.OperationDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Choose random operation
			opIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(operations))))
			if err != nil {
				log.Printf("Error generating random number: %v", err)
				continue
			}

			operation := operations[opIndex.Int64()]
			if err := operation(); err != nil {
				m.stats.ErrorOps++
				fmt.Printf("[ERROR] Operation failed: %v\n", err)
			}
		}
	}
}

func (m *MinioClient) writeOperation() error {
	bucket, err := m.getRandomBucket()
	if err != nil {
		return fmt.Errorf("failed to get random bucket: %v", err)
	}

	objectName := m.generateObjectName()
	content := m.generateRandomContent()

	ctx := context.Background()
	_, err = m.client.PutObject(ctx, bucket, objectName,
		strings.NewReader(content), int64(len(content)), minio.PutObjectOptions{})

	if err != nil {
		return fmt.Errorf("write operation failed: %v", err)
	}

	m.stats.WriteOps++
	fmt.Printf("[SUCCESS] WRITE: %s/%s (%d bytes)\n", bucket, objectName, len(content))
	return nil
}

func (m *MinioClient) readOperation() error {
	// List objects and pick one randomly
	objects, err := m.listObjects()
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		// No objects to read, create one first
		return m.writeOperation()
	}

	// Pick random object
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(objects))))
	if err != nil {
		return err
	}

	objectInfo := objects[index.Int64()]
	ctx := context.Background()

	obj, err := m.client.GetObject(ctx, objectInfo.Bucket, objectInfo.Key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("read operation failed: %v", err)
	}
	defer obj.Close()

	// Read the content
	content, err := io.ReadAll(obj)
	if err != nil {
		return fmt.Errorf("read operation failed to read content: %v", err)
	}

	m.stats.ReadOps++
	fmt.Printf("[SUCCESS] READ: %s/%s (%d bytes)\n", objectInfo.Bucket, objectInfo.Key, len(content))
	return nil
}

func (m *MinioClient) overwriteOperation() error {
	// List objects and pick one randomly
	objects, err := m.listObjects()
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		// No objects to overwrite, create one first
		return m.writeOperation()
	}

	// Pick random object
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(objects))))
	if err != nil {
		return err
	}

	objectInfo := objects[index.Int64()]
	content := m.generateRandomContent()

	ctx := context.Background()
	_, err = m.client.PutObject(ctx, objectInfo.Bucket, objectInfo.Key,
		strings.NewReader(content), int64(len(content)), minio.PutObjectOptions{})

	if err != nil {
		return fmt.Errorf("overwrite operation failed: %v", err)
	}

	m.stats.OverwriteOps++
	fmt.Printf("[SUCCESS] OVERWRITE: %s/%s (%d bytes)\n", objectInfo.Bucket, objectInfo.Key, len(content))
	return nil
}

func (m *MinioClient) deleteOperation() error {
	// List objects and pick one randomly
	objects, err := m.listObjects()
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		// No objects to delete, create one first then delete it
		if err := m.writeOperation(); err != nil {
			return err
		}
		// Refresh objects list
		objects, err = m.listObjects()
		if err != nil {
			return err
		}
	}

	// Pick random object
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(objects))))
	if err != nil {
		return err
	}

	objectInfo := objects[index.Int64()]
	ctx := context.Background()

	err = m.client.RemoveObject(ctx, objectInfo.Bucket, objectInfo.Key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete operation failed: %v", err)
	}

	m.stats.DeleteOps++
	fmt.Printf("[SUCCESS] DELETE: %s/%s\n", objectInfo.Bucket, objectInfo.Key)
	return nil
}

func (m *MinioClient) prefixDeleteOperation() error {
	// Get all objects across all buckets
	objects, err := m.listObjects()
	if err != nil {
		return fmt.Errorf("failed to list objects for prefix deletion: %v", err)
	}

	if len(objects) == 0 {
		// No objects to delete, create some first
		return m.writeOperation()
	}

	// Group objects by their prefix (first 2-3 levels of directory structure) within each bucket
	prefixGroups := make(map[string][]ObjectInfo)
	for _, objectInfo := range objects {
		// Extract prefix (up to 2nd or 3rd slash)
		parts := strings.Split(objectInfo.Key, "/")
		if len(parts) >= 2 {
			// Use bucket and first 2 levels as prefix for deletion
			prefix := objectInfo.Bucket + ":" + strings.Join(parts[:2], "/") + "/"
			prefixGroups[prefix] = append(prefixGroups[prefix], objectInfo)
		}
	}

	if len(prefixGroups) == 0 {
		return fmt.Errorf("no valid prefixes found for deletion")
	}

	// Select a random prefix that has multiple objects (for better demo)
	var selectedPrefix string
	var objectsToDelete []ObjectInfo
	maxObjects := 0

	for prefix, prefixObjects := range prefixGroups {
		if len(prefixObjects) > maxObjects {
			maxObjects = len(prefixObjects)
			selectedPrefix = prefix
			objectsToDelete = prefixObjects
		}
	}

	// If no prefix has multiple objects, just pick any prefix
	if selectedPrefix == "" {
		for prefix, prefixObjects := range prefixGroups {
			selectedPrefix = prefix
			objectsToDelete = prefixObjects
			break
		}
	}

	ctx := context.Background()
	deletedCount := 0

	// Delete all objects under the selected prefix
	for _, objectInfo := range objectsToDelete {
		err = m.client.RemoveObject(ctx, objectInfo.Bucket, objectInfo.Key, minio.RemoveObjectOptions{})
		if err != nil {
			fmt.Printf("[ERROR] Failed to delete %s/%s: %v\n", objectInfo.Bucket, objectInfo.Key, err)
			continue
		}
		deletedCount++
	}

	m.stats.PrefixDeleteOps++
	fmt.Printf("[SUCCESS] PREFIX DELETE: %s (%d objects deleted)\n", selectedPrefix, deletedCount)
	return nil
}

func (m *MinioClient) multipartWriteOperation() error {
	bucket, err := m.getRandomBucket()
	if err != nil {
		return fmt.Errorf("failed to get random bucket: %v", err)
	}

	objectName := m.generateMultipartObjectName()

	ctx := context.Background()

	// Generate larger content to force multipart upload (must be >64MB for guaranteed multipart)
	contentSize := 70 * 1024 * 1024 // 70MB to ensure multipart upload
	content := m.generateVeryLargeContent(contentSize)

	// Use PutObject with small part size to force multipart behavior
	_, err = m.client.PutObject(ctx, bucket, objectName,
		strings.NewReader(content), int64(len(content)),
		minio.PutObjectOptions{
			PartSize: 5 * 1024 * 1024, // 5MB parts - forces multipart
		})

	if err != nil {
		return fmt.Errorf("multipart write operation failed: %v", err)
	}

	m.stats.MultipartOps++
	fmt.Printf("[SUCCESS] MULTIPART WRITE: %s/%s (%d MB, multipart forced)\n", bucket, objectName, len(content)/(1024*1024))
	return nil
}

func (m *MinioClient) listObjects() ([]ObjectInfo, error) {
	ctx := context.Background()
	var objects []ObjectInfo
	buckets := m.parseBuckets()

	// List all objects across all buckets
	for _, bucket := range buckets {
		objectCh := m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
			Recursive: true,
		})

		for object := range objectCh {
			if object.Err != nil {
				return nil, object.Err
			}
			// Filter objects that contain our base prefix anywhere in the path
			if strings.Contains(object.Key, m.config.ObjectPrefix) {
				objects = append(objects, ObjectInfo{
					Bucket: bucket,
					Key:    object.Key,
				})
			}
		}
	}

	return objects, nil
}

// ObjectInfo represents an object with its bucket information
type ObjectInfo struct {
	Bucket string
	Key    string
}

func (m *MinioClient) generateRandomPrefix() string {
	// Generate random prefix like: data/2025/09/30/ or logs/batch-001/ or temp/user-xyz/
	prefixTypes := [][]string{
		{"data", "logs", "backup", "temp", "cache", "media"},
		{"2025", "2024", "2023", "batch-001", "batch-002", "user-001", "user-002", "session-a", "session-b"},
		{"09", "10", "11", "q1", "q2", "q3", "daily", "weekly", "monthly"},
		{"30", "01", "15", "prod", "test", "dev", "staging"},
	}

	var pathParts []string
	for _, typeGroup := range prefixTypes {
		if len(typeGroup) > 0 {
			index, _ := rand.Int(rand.Reader, big.NewInt(int64(len(typeGroup))))
			pathParts = append(pathParts, typeGroup[index.Int64()])
		}
	}

	// Randomly choose 2-4 parts to create varied depth
	depth, _ := rand.Int(rand.Reader, big.NewInt(3))
	depth = depth.Add(depth, big.NewInt(2)) // 2-4 parts

	if int(depth.Int64()) > len(pathParts) {
		depth = big.NewInt(int64(len(pathParts)))
	}

	selectedParts := pathParts[:depth.Int64()]
	return strings.Join(selectedParts, "/") + "/"
}

func (m *MinioClient) generateObjectName() string {
	randomPrefix := m.generateRandomPrefix()
	now := time.Now()
	timestamp := fmt.Sprintf("%s-%03d", now.Format("2006-01-02T15-04-05"), now.Nanosecond()/1000000)
	randomNum, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%s%s-%s-%d", randomPrefix, m.config.ObjectPrefix, timestamp, randomNum.Int64())
}

func (m *MinioClient) generateMultipartObjectName() string {
	randomPrefix := m.generateRandomPrefix()
	now := time.Now()
	timestamp := fmt.Sprintf("%s-%03d", now.Format("2006-01-02T15-04-05"), now.Nanosecond()/1000000)
	randomNum, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%s%s-%s-%d-m", randomPrefix, m.config.ObjectPrefix, timestamp, randomNum.Int64())
}

func (m *MinioClient) generateRandomContent() string {
	sizes := []int{100, 500, 1024, 2048, 5120} // Different content sizes
	sizeIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(sizes))))
	size := sizes[sizeIndex.Int64()]

	content := make([]byte, size)
	for i := range content {
		char, _ := rand.Int(rand.Reader, big.NewInt(26))
		content[i] = byte('a' + char.Int64())
	}

	return string(content)
}

func (m *MinioClient) generateVeryLargeContent(size int) string {
	// Generate very large content for guaranteed multipart uploads
	content := make([]byte, size)

	// Use a more efficient approach for very large content
	pattern := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	patternLen := len(pattern)

	for i := 0; i < size; i++ {
		content[i] = pattern[i%patternLen]
	}

	return string(content)
}

func (m *MinioClient) printStats(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Printf("\n[STATS] Read=%d, Write=%d, Overwrite=%d, Delete=%d, PrefixDel=%d, Multipart=%d, Errors=%d\n",
				m.stats.ReadOps, m.stats.WriteOps, m.stats.OverwriteOps, m.stats.DeleteOps, m.stats.PrefixDeleteOps, m.stats.MultipartOps, m.stats.ErrorOps)
		}
	}
}

func (m *MinioClient) printFinalStats() {
	total := m.stats.ReadOps + m.stats.WriteOps + m.stats.OverwriteOps + m.stats.DeleteOps + m.stats.PrefixDeleteOps + m.stats.MultipartOps
	fmt.Printf("Read Operations:         %d\n", m.stats.ReadOps)
	fmt.Printf("Write Operations:        %d\n", m.stats.WriteOps)
	fmt.Printf("Overwrite Operations:    %d\n", m.stats.OverwriteOps)
	fmt.Printf("Delete Operations:       %d\n", m.stats.DeleteOps)
	fmt.Printf("Prefix Delete Operations:%d\n", m.stats.PrefixDeleteOps)
	fmt.Printf("Multipart Operations:    %d\n", m.stats.MultipartOps)
	fmt.Printf("Error Operations:        %d\n", m.stats.ErrorOps)
	fmt.Printf("Total Operations:        %d\n", total)
}
