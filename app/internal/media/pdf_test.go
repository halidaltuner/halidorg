package media

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/store/storetest"
)

// A PDF uploads as a document: no dimensions, not offered as an image.
func TestUploadPDF(t *testing.T) {
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t), storetest.NewBlob())
	it, err := r.Upload(ctx, "My CV.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n"))
	if err != nil || it.Mime != "application/pdf" || it.IsImage() || it.Width != nil || it.R2Key != "media/"+it.ID+"/my-cv.pdf" {
		t.Fatalf("pdf %+v %v", it, err)
	}
	if _, err := r.SetAlt(ctx, it.ID, "Özgeçmiş", "CV"); err != nil {
		t.Fatal(err)
	}
	if ready, _ := r.Ready(ctx); len(ready) != 0 {
		t.Error("a PDF offered to image pickers")
	}
	if err := r.CheckCover(ctx, it.ID); !errors.Is(err, ErrType) {
		t.Errorf("PDF as a cover: %v", err)
	}
	if _, err := r.Upload(ctx, "x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); !errors.Is(err, ErrType) {
		t.Errorf("svg: %v", err)
	}
}
