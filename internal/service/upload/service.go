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

// ─── Folder helpers ───────────────────────────────────────────────────────────
//
// All files are stored under the root "ogaos/" namespace in ImageKit.
// Per-business and per-product sub-folders keep everything organised
// and make it easy to find or delete all assets for a specific entity.
//
// Structure:
//   ogaos/businesses/{businessID}/logo/
//   ogaos/businesses/{businessID}/gallery/
//   ogaos/products/{productID}/cover/
//   ogaos/products/{productID}/gallery/
//   ogaos/products/{productID}/file/       ← private
//   ogaos/applications/{applicationID}/cv/ ← private
//   ogaos/receipts/

const root = "ogaos"

func bizLogoFolder(businessID uuid.UUID) string {
	return fmt.Sprintf("%s/businesses/%s/logo", root, businessID.String())
}
func bizGalleryFolder(businessID uuid.UUID) string {
	return fmt.Sprintf("%s/businesses/%s/gallery", root, businessID.String())
}
func productCoverFolder(productID uuid.UUID) string {
	return fmt.Sprintf("%s/products/%s/cover", root, productID.String())
}
func productGalleryFolder(productID uuid.UUID) string {
	return fmt.Sprintf("%s/products/%s/gallery", root, productID.String())
}
func productFileFolder(productID uuid.UUID) string {
	return fmt.Sprintf("%s/products/%s/file", root, productID.String())
}
func physicalProductImageFolder(productID uuid.UUID) string {
	return fmt.Sprintf("%s/products/%s/image", root, productID.String())
}
func cvFolder(applicationID uuid.UUID) string {
	return fmt.Sprintf("%s/applications/%s/cv", root, applicationID.String())
}

const receiptFolder = root + "/receipts"

// ─── Result ───────────────────────────────────────────────────────────────────

type UploadResult struct {
	URL      string
	FileID   string
	FileSize int64
	MimeType string
}

// ─── Business uploads ─────────────────────────────────────────────────────────

// UploadLogo uploads the business logo.
// Stored at: ogaos/businesses/{businessID}/logo/logo{ext}
func (s *Service) UploadLogo(businessID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("logo must be an image file (jpg, png, webp)")
	}
	fileName := fmt.Sprintf("logo%s", ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    bizLogoFolder(businessID),
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("logo upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// UploadBusinessGalleryImage uploads one storefront gallery photo (max 3).
// Stored at: ogaos/businesses/{businessID}/gallery/gallery-{timestamp}{ext}
func (s *Service) UploadBusinessGalleryImage(businessID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("gallery image must be jpg, png, or webp")
	}
	fileName := fmt.Sprintf("gallery-%d%s", time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    bizGalleryFolder(businessID),
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("business gallery upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// ─── Physical product uploads ─────────────────────────────────────────────────

// UploadProductImage uploads a physical (inventory) product image.
// Stored at: ogaos/products/{productID}/image/image-{timestamp}{ext}
func (s *Service) UploadProductImage(productID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("product image must be an image file (jpg, png, webp)")
	}
	fileName := fmt.Sprintf("image-%d%s", time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    physicalProductImageFolder(productID),
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("product image upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// ─── Digital product uploads ──────────────────────────────────────────────────

// UploadCoverImage uploads the digital product cover image.
// Stored at: ogaos/products/{productID}/cover/cover{ext}
func (s *Service) UploadCoverImage(productID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("cover image must be an image file (jpg, png, webp)")
	}
	fileName := fmt.Sprintf("cover%s", ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    productCoverFolder(productID),
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("cover image upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// UploadProductGalleryImage uploads one of up to 3 digital product gallery images.
// Stored at: ogaos/products/{productID}/gallery/gallery-{timestamp}{ext}
func (s *Service) UploadProductGalleryImage(productID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if !isImageExt(ext) {
		return nil, errors.New("gallery image must be jpg, png, or webp")
	}
	fileName := fmt.Sprintf("gallery-%d%s", time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    productGalleryFolder(productID),
		IsPrivate: false,
	})
	if err != nil {
		return nil, fmt.Errorf("product gallery upload failed: %w", err)
	}
	return &UploadResult{URL: resp.URL, FileID: resp.FileID, FileSize: int64(resp.Size)}, nil
}

// UploadDigitalProductFile uploads the private downloadable file for a digital product.
// Stored at: ogaos/products/{productID}/file/{filename}
// PRIVATE — access requires a signed URL, never expose directly.
func (s *Service) UploadDigitalProductFile(productID uuid.UUID, data []byte, originalName, mimeType string) (*UploadResult, error) {
	ext := safeExt(originalName)
	fileName := fmt.Sprintf("file-%d%s", time.Now().UnixMilli(), ext)
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  fileName,
		Folder:    productFileFolder(productID),
		IsPrivate: true, // ← critical: signed URL required to access
	})
	if err != nil {
		return nil, fmt.Errorf("digital product file upload failed: %w", err)
	}
	return &UploadResult{
		URL:      resp.FilePath, // store path, not public URL
		FileID:   resp.FileID,
		FileSize: int64(resp.Size),
		MimeType: mimeType,
	}, nil
}

// ─── Other uploads ────────────────────────────────────────────────────────────

// UploadCV uploads a job applicant's CV as a private PDF.
// Stored at: ogaos/applications/{applicationID}/cv/cv.pdf
func (s *Service) UploadCV(applicationID uuid.UUID, data []byte, originalName string) (*UploadResult, error) {
	ext := safeExt(originalName)
	if ext != ".pdf" {
		return nil, errors.New("CV must be a PDF file")
	}
	resp, err := s.client.Upload(imagekit.UploadRequest{
		File:      data,
		FileName:  "cv.pdf",
		Folder:    cvFolder(applicationID),
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
