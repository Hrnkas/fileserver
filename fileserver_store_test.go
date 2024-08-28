package fileserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to the test database: %v", err)
	}

	err = db.AutoMigrate(&Upload{}, &Part{})
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

func TestStore(t *testing.T) {
	db := setupTestDB(t)
	os.MkdirAll("/tmp/", os.ModePerm)
	fs, _ := NewFileserver("/tmp", db, func(w http.ResponseWriter, req *http.Request) bool { return true })

	// Prepare the upload entry in the database
	upload := Upload{Code: "testcode", Filename: "testfile"}
	db.Create(&upload)

	t.Run("Valid request", func(t *testing.T) {

		req := httptest.NewRequest(http.MethodPut, "/upload/testcode/part1", strings.NewReader("file content"))
		req.SetPathValue("code", "testcode")
		req.SetPathValue("part", "part1")

		w := httptest.NewRecorder()

		fs.Store(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		partFile := filepath.Join("/tmp", "testcode.part1")
		defer os.Remove(partFile) // clean up

		_, err := os.Stat(partFile)
		assert.NoError(t, err, "The file should have been created")

		var part Part
		err = db.First(&part, "part_code = ? AND upload_id = ?", "part1", upload.ID).Error
		assert.NoError(t, err, "The part should have been recorded in the database")
	})

	t.Run("Missing code_upload or code_part", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/upload//part1", strings.NewReader("file content"))
		req.SetPathValue("part", "part1")
		w := httptest.NewRecorder()

		fs.Store(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "Parameters must not be empty.", strings.TrimSpace(w.Body.String()))
	})

	t.Run("Upload not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/upload/unknowncode/part1", strings.NewReader("file content"))
		req.SetPathValue("code", "unknowncode")
		req.SetPathValue("part", "part1")
		w := httptest.NewRecorder()

		fs.Store(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Equal(t, "Upload with given code could not be found.", strings.TrimSpace(w.Body.String()))
	})

}
func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	os.MkdirAll("/tmp/", os.ModePerm)
	fs, _ := NewFileserver("/tmp", db, func(w http.ResponseWriter, req *http.Request) bool { return true })

	// Prepare the upload entry in the database
	upload := Upload{Code: "testcode", Filename: "testfile"}
	db.Create(&upload)

	t.Run("Second insert", func(t *testing.T) {

		req := httptest.NewRequest(http.MethodDelete, "/delete/testcode", strings.NewReader(""))
		req.SetPathValue("code", "testcode")

		w := httptest.NewRecorder()

		fs.DeleteUpload(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		//try to insert again
		upload := Upload{Code: "testcode", Filename: "testfile"}
		errCreate := db.Create(&upload).Error
		assert.NoError(t, errCreate, "second create should have been successful")
	})

}
