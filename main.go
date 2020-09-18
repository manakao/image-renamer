package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/rwcarlsen/goexif/exif"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	sourceDirectory      string
	destinationDirectory string
)

func init() {
	flag.StringVar(&sourceDirectory, "from", "", "Место хранения не отсортированных изображений")
	flag.StringVar(&destinationDirectory, "to", "", "Место хранения отсортированных изображений")

	flag.Parse()
}

func main() {
	if err := NewImageMover(sourceDirectory, destinationDirectory).Move(); err != nil {
		fmt.Println(err)
	}
}

type ImageMover struct {
	sourceDirectory      string
	destinationDirectory string

	files      chan string
	filesCount uint64

	images      chan Image
	imagesCount uint64

	movedCount uint64

	wg *sync.WaitGroup
}

func NewImageMover(sourceDirectory, destinationDirectory string) *ImageMover {
	return &ImageMover{
		sourceDirectory:      sourceDirectory,
		destinationDirectory: destinationDirectory,

		files:  make(chan string),
		images: make(chan Image),
		wg:     &sync.WaitGroup{},
	}
}

func (m *ImageMover) Move() error {

	if err := m.validate(); err != nil {
		return err
	}

	go m.scan()
	go m.makeImages()
	go m.moveImages()

	m.wg.Add(3)
	m.wg.Wait()

	return nil
}

func (m *ImageMover) validate() error {
	if len(m.sourceDirectory) == 0 {
		return errors.New("место хранения не отсортированных изображений не указано")
	}
	if len(m.destinationDirectory) == 0 {
		return errors.New("место хранения отсортированных изображений не указано")
	}

	stat, err := os.Stat(m.sourceDirectory)

	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return errors.New("папка с изображениями не указана")
	}

	stat, err = os.Stat(m.destinationDirectory)

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *ImageMover) scan() {
	defer m.wg.Done()

	fn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		m.files <- path
		m.filesCount++

		return nil
	}

	if err := filepath.Walk(m.sourceDirectory, fn); err != nil {
		log.Fatalln(err)
	}

	close(m.files)

	fmt.Printf("Всего файлов: %d\n", m.filesCount)
}

func (m *ImageMover) makeImages() {
	defer m.wg.Done()

	for path := range m.files {
		f, err := os.Open(path)

		if err != nil {
			log.Fatalln(err)
		}

		var decodeInfo *exif.Exif
		decodeInfo, err = exif.Decode(f)

		if err != nil {
			continue
		}

		var dateTime time.Time
		dateTime, err = decodeInfo.DateTime()

		if err != nil {
			continue
		}

		ext := filepath.Ext(strings.ToLower(f.Name()))

		// интересуют только jpeg
		switch ext {
		case ".jpg", ".jpeg":
		default:
			continue
		}

		image := NewImage(path, ext, dateTime)

		m.images <- image
		m.imagesCount++

		f.Close()
	}

	close(m.images)

	fmt.Printf("Распознано картинок: %d\n", m.imagesCount)
}

func (m *ImageMover) moveImages() {
	defer m.wg.Done()

	for image := range m.images {

		destination := filepath.Join(m.destinationDirectory, image.ToPath)
		dir := filepath.Dir(destination)

		if err := os.MkdirAll(dir, os.ModePerm); err != nil && !os.IsExist(err) {
			log.Fatalln(err)
		}

		if err := os.Rename(image.FromPath, destination); err != nil {
			log.Fatalln(err)
		}

		m.movedCount++
	}

	fmt.Printf("Перемещено картинок: %d\n", m.movedCount)
}

type Image struct {
	FromPath  string
	ToPath    string
	Extension string
	CreatedAt time.Time
}

func NewImage(path, extension string, createdAt time.Time) Image {
	year, month, day := createdAt.Date()
	hour, min, sec := createdAt.Clock()

	dir := fmt.Sprintf("%02d", year)
	file := fmt.Sprintf("%02d.%02d.%02d_%02d.%02d.%02d", year, month, day, hour, min, sec) + extension

	return Image{
		FromPath:  path,
		Extension: extension,
		CreatedAt: createdAt,
		ToPath:    filepath.Join(dir, file),
	}
}
