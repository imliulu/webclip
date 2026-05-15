package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gogf/gf/v2/frame/g"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// Storage 对象存储统一接口
type Storage interface {
	MaxFileSize() int64
	PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	PresignGetURL(ctx context.Context, key string) (string, error)
	DeleteObject(ctx context.Context, key string) error
	DeleteObjectsByPrefix(ctx context.Context, prefix string) error
}

// Default 全局 Storage 实例；未配置时为 nil
var Default Storage

// ---- 工厂 ----

// NewStorage 根据配置创建 Storage 实例；endpoint 为空时返回 nil
func NewStorage(ctx context.Context) (Storage, error) {
	cfg := g.Cfg()
	endpoint := cfg.MustGet(ctx, "s3.endpoint", "").String()
	if endpoint == "" {
		g.Log().Info(ctx, "s3 endpoint not configured, file sharing disabled")
		return nil, nil
	}

	vendor := cfg.MustGet(ctx, "s3.vendor", "").String()
	switch strings.ToLower(vendor) {
	case "cos", "tencent":
		return newCOSStorage(ctx)
	default:
		return newS3Storage(ctx)
	}
}

// ---- S3 实现 (AWS / MinIO / 其他兼容) ----

// S3Storage 基于 AWS SDK v2 的 S3 兼容存储
type S3Storage struct {
	client      *s3.Client
	presigner   *s3.PresignClient
	bucket      string
	publicURL   string
	maxFileSize int64
}

func newS3Storage(ctx context.Context) (*S3Storage, error) {
	cfg := g.Cfg()
	endpoint := cfg.MustGet(ctx, "s3.endpoint", "").String()
	region := cfg.MustGet(ctx, "s3.region", "us-east-1").String()
	bucket := cfg.MustGet(ctx, "s3.bucket", "webclip").String()
	accessKey := envOrConfig(ctx, "S3_ACCESS_KEY", "s3.accessKey")
	secretKey := envOrConfig(ctx, "S3_SECRET_KEY", "s3.secretKey")
	publicURL := cfg.MustGet(ctx, "s3.publicUrl", "").String()
	maxFileSize := cfg.MustGet(ctx, "s3.maxFileSize", int64(524288000)).Int64()
	pathStyle := cfg.MustGet(ctx, "s3.pathStyle", false).Bool()

	g.Log().Infof(ctx, "s3 (aws) connecting: endpoint=%s bucket=%s region=%s pathStyle=%v", endpoint, bucket, region, pathStyle)

	s3Config := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(_, _ string, _ ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           endpoint,
				SigningRegion: region,
				Source:        aws.EndpointSourceCustom,
			}, nil
		}),
	}

	client := s3.NewFromConfig(s3Config, func(o *s3.Options) {
		o.UsePathStyle = pathStyle
	})
	presigner := s3.NewPresignClient(client, func(o *s3.PresignOptions) {
		o.Expires = 30 * time.Minute
	})

	// 验证连接（失败仅警告）
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		g.Log().Warningf(ctx, "s3 HeadBucket warning (non-fatal): %v", err)
	} else {
		g.Log().Info(ctx, "s3 HeadBucket ok")
	}

	return &S3Storage{
		client:      client,
		presigner:   presigner,
		bucket:      bucket,
		publicURL:   publicURL,
		maxFileSize: maxFileSize,
	}, nil
}

func (s *S3Storage) MaxFileSize() int64 { return s.maxFileSize }

func (s *S3Storage) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put object failed: %w", err)
	}
	return nil
}

func (s *S3Storage) PresignGetURL(ctx context.Context, key string) (string, error) {
	if s.publicURL != "" {
		return fmt.Sprintf("%s/%s/%s", s.publicURL, s.bucket, key), nil
	}
	req, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = 1 * time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("s3 presign get failed: %w", err)
	}
	return req.URL, nil
}

func (s *S3Storage) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete object failed: %w", err)
	}
	return nil
}

func (s *S3Storage) DeleteObjectsByPrefix(ctx context.Context, prefix string) error {
	lister := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for lister.HasMorePages() {
		page, err := lister.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("s3 list objects failed: %w", err)
		}
		if len(page.Contents) == 0 {
			return nil
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}
		_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("s3 batch delete failed: %w", err)
		}
	}
	return nil
}

// ---- COS 实现 (腾讯云) ----

// COSStorage 基于腾讯云 COS SDK 的存储实现
type COSStorage struct {
	client      *cos.Client
	bucketURL   string // https://bucket-appid.cos.region.myqcloud.com
	publicURL   string
	maxFileSize int64
	accessKey   string // SecretId，用于预签名
	secretKey   string // SecretKey，用于预签名
}

func newCOSStorage(ctx context.Context) (*COSStorage, error) {
	cfg := g.Cfg()
	endpoint := cfg.MustGet(ctx, "s3.endpoint", "").String()
	bucket := cfg.MustGet(ctx, "s3.bucket", "webclip").String()
	accessKey := envOrConfig(ctx, "S3_ACCESS_KEY", "s3.accessKey")
	secretKey := envOrConfig(ctx, "S3_SECRET_KEY", "s3.secretKey")
	publicURL := cfg.MustGet(ctx, "s3.publicUrl", "").String()
	maxFileSize := cfg.MustGet(ctx, "s3.maxFileSize", int64(524288000)).Int64()

	// COS endpoint 格式: https://cos.ap-shanghai.myqcloud.com
	// Bucket URL 格式: https://bucket-appid.cos.ap-shanghai.myqcloud.com
	bucketURLStr := fmt.Sprintf("https://%s.%s", bucket, strings.TrimPrefix(endpoint, "https://"))
	bucketURLStr = strings.TrimRight(bucketURLStr, "/")

	g.Log().Infof(ctx, "cos connecting: endpoint=%s bucket=%s bucketURL=%s", endpoint, bucket, bucketURLStr)

	u, err := url.Parse(bucketURLStr)
	if err != nil {
		return nil, fmt.Errorf("cos parse bucket url failed: %w", err)
	}

	client := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  accessKey,
			SecretKey: secretKey,
		},
	})

	// 验证连接（失败仅警告）
	if _, err := client.Bucket.Head(ctx, nil); err != nil {
		g.Log().Warningf(ctx, "cos HeadBucket warning (non-fatal): %v", err)
	} else {
		g.Log().Info(ctx, "cos HeadBucket ok")
	}

	return &COSStorage{
		client:      client,
		bucketURL:   bucketURLStr,
		publicURL:   publicURL,
		maxFileSize: maxFileSize,
		accessKey:   accessKey,
		secretKey:   secretKey,
	}, nil
}

func (s *COSStorage) MaxFileSize() int64 { return s.maxFileSize }

func (s *COSStorage) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			ContentLength: size,
		},
	}
	_, err := s.client.Object.Put(ctx, key, reader, opt)
	if err != nil {
		return fmt.Errorf("cos put object failed: %w", err)
	}
	return nil
}

func (s *COSStorage) PresignGetURL(ctx context.Context, key string) (string, error) {
	if s.publicURL != "" {
		return fmt.Sprintf("%s/%s", s.publicURL, key), nil
	}
	presignedURL, err := s.client.Object.GetPresignedURL(ctx, http.MethodGet, key, s.accessKey, s.secretKey, 1*time.Hour, nil)
	if err != nil {
		return "", fmt.Errorf("cos presign get failed: %w", err)
	}
	return presignedURL.String(), nil
}

func (s *COSStorage) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.Object.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("cos delete object failed: %w", err)
	}
	return nil
}

func (s *COSStorage) DeleteObjectsByPrefix(ctx context.Context, prefix string) error {
	getOpt := &cos.BucketGetOptions{Prefix: prefix, MaxKeys: 1000}
	for {
		res, _, err := s.client.Bucket.Get(ctx, getOpt)
		if err != nil {
			return fmt.Errorf("cos list objects failed: %w", err)
		}
		if len(res.Contents) == 0 {
			return nil
		}
		objects := make([]cos.Object, 0, len(res.Contents))
		for _, obj := range res.Contents {
			objects = append(objects, cos.Object{Key: obj.Key})
		}
		delOpt := &cos.ObjectDeleteMultiOptions{Objects: objects}
		if _, _, err := s.client.Object.DeleteMulti(ctx, delOpt); err != nil {
			return fmt.Errorf("cos batch delete failed: %w", err)
		}
		if !res.IsTruncated {
			return nil
		}
		getOpt.Marker = res.NextMarker
	}
}

// ---- 工具函数 ----

// envOrConfig 优先从环境变量读取，为空则从配置文件读取
func envOrConfig(ctx context.Context, envKey, configKey string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return g.Cfg().MustGet(ctx, configKey, "").String()
}
