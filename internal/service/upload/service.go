// internal/service/upload/service.go
package upload

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"ogaos-backend/internal/external/imagekit"
)

type Service struct {
	client *imagekit.Client
}

func NewService(client *imagekit.Client) *Service {
	return &Service{client: client}
}

// ─── Folders ─────────────────────────────────────────────────────────────────

const (
	FolderLogos         = "logos"
	FolderProductImages = "products"
	FolderDigitalFiles  = "digital-products"
	FolderCoverImages   = "covers"
	FolderReceiptPDFs   = "documents"
	FolderCVs           = "cvs"
)

// ─── Result ───────────────────────────────────────────────────────────────────

type UploadResult struct {
	URL      string
	FileID   string
	FileSize int64
	MimeType string
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// UploadLogo uploads a business logo as a public image.
// Returns the public URL to store in business.logo_url.
func (s *Service) UploadLogo(businessID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("logo must be an image file (jpg, png, webp)")
	}
	fileName := fmt.Sprintf("logo-%s%s", businessID.String(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    FolderLogos,
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("logo upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// UploadProductImage uploads a physical product image as a public image.
func (s *Service) UploadProductImage(productID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("product image must be an image file (jpg, png, webp)")
	}
	fileName := fmt.Sprintf("product-%s-%d%s", productID.String()[:8], time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    FolderProductImages,
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("product image upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// UploadDigitalProductFile uploads the private downloadable file for a digital product.
// The file is stored as private — access requires a signed URL.
func (s *Service) UploadDigitalProductFile(productID uuid.UUID, data []byte, originalName, mimeType string) (*UploadResult, error) {
	ext := safeExt(originalName)
	fileName := fmt.Sprintf("dp-%s-%d%s", productID.String()[:8], time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    FolderDigitalFiles,
		IsPrivate: true, // ← critical: private file, requires signed URL
	})
	if err != nil {
		return nil, fmt.Errorf("digital product file upload failed: %w", err)
	}
	return &UploadResult{
		URL:      resp.FilePath, // store the path, not the public URL
		FileID:   resp.FileID,
		FileSize: int64(resp.Size),
		MimeType: mimeType,
	}, nil
}

// UploadCoverImage uploads a digital product cover image as a public image.
func (s *Service) UploadCoverImage(productID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("cover image must be an image file (jpg, png, webp)")
	}
	fileName := fmt.Sprintf("cover-%s-%d%s", productID.String()[:8], time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    FolderCoverImages,
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("cover image upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// UploadCV uploads a job applicant's CV. Stored as private.
func (s *Service) UploadCV(applicationID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if ext != ".pdf" {
		return nil, errors.New("CV must be a PDF file")
	}
	fileName := fmt.Sprintf("cv-%s%s", applicationID.String()[:8], ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    FolderCVs,
		IsPrivate: true,
	})
	if err != nil {
		return nil, fmt.Errorf("CV upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func safeExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return ".bin"
	}
	return ext
}

func isImageExt(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif"
}
