package main

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/fogleman/imview"
	"github.com/nfnt/resize"
)

func Width(i image.Image) int {
	return i.Bounds().Max.X - i.Bounds().Min.X
}

func Height(i image.Image) int {
	return i.Bounds().Max.Y - i.Bounds().Min.Y
}

type MyImage struct {
	value *image.RGBA
}

func (i *MyImage) Set(x, y int, c color.Color) {
	i.value.Set(x, y, c)
}

func (i *MyImage) ColorModel() color.Model {
	return i.value.ColorModel()
}

func (i *MyImage) Bounds() image.Rectangle {
	return i.value.Bounds()
}

func (i *MyImage) At(x, y int) color.Color {
	return i.value.At(x, y)
}

type Circle struct {
	p image.Point
	r int
}

func (c *Circle) ColorModel() color.Model {
	return color.AlphaModel
}

func (c *Circle) Bounds() image.Rectangle {
	return image.Rect(c.p.X-int(c.r), c.p.Y-int(c.r), c.p.X+int(c.r), c.p.Y+int(c.r))
}

func (c *Circle) At(x, y int) color.Color {
	xx, yy, rr := float64(x-c.p.X)+0.5, float64(y-c.p.Y)+0.5, float64(c.r)
	if xx*xx+yy*yy < rr*rr {
		return color.Alpha{255}
	}
	return color.Alpha{0}
}

type Size struct {
	width  uint
	height uint
}

type ImageShape string

type ImagePositionAndSize struct {
	sp   image.Point
	size Size
}

const (
	RectangleShape   ImageShape = "Rectangle"
	CircleShape      ImageShape = "Circle"
	CircleDiameter              = 0.8
	RectanglePadding            = 1
	CirclePadding               = 20
)

func drawLine(img *image.RGBA, line_width int, space_from_end_x int, space_from_end_y int) {
	for i := img.Bounds().Max.X - line_width - space_from_end_x; i < img.Bounds().Max.X-space_from_end_x; i++ {
		img.Set(i, img.Bounds().Max.Y-space_from_end_y, color.RGBA{255, 255, 255, 255})
	}
}

func (bgImg *MyImage) drawRaw(innerImg image.Image, sp image.Point, width uint, height uint) {
	resizedImg := resize.Resize(width, height, innerImg, resize.Lanczos3)
	w := int(Width(resizedImg))
	h := int(Height(resizedImg))
	draw.Draw(bgImg, image.Rectangle{sp, image.Point{sp.X + w, sp.Y + h}}, resizedImg, image.ZP, draw.Src)
}

func (bgImg *MyImage) drawInCircle(innerImg image.Image, sp image.Point, width uint, height uint, diameter int) {
	resizedImg := resize.Resize(width, height, innerImg, resize.Lanczos3)

	r := diameter
	if r > Width(resizedImg) {
		r = Width(resizedImg)
	}

	if r > Height(resizedImg) {
		r = int(Height(resizedImg))
	}

	mask := &Circle{image.Point{Width(resizedImg) / 2, Height(resizedImg) / 2}, r / 2}

	draw.DrawMask(bgImg, image.Rectangle{sp, image.Point{sp.X + Width(resizedImg), sp.Y + Height(resizedImg)}}, resizedImg, image.ZP, mask, image.ZP, draw.Over)
}

func makeImageCollage(desiredWidth int, desiredHeight int, numberOfRows int, shape ImageShape, images ...image.Image) *MyImage {

	sort.Slice(images, func(i, j int) bool {
		return Height(images[i]) > Height(images[j])
	})

	numberOfColumns := len(images) / numberOfRows
	imagesMatrix := make([][]image.Image, numberOfRows)

	numberOfColumnsAdded := 0
	maxNumberOfColumns := 0
	for idx := 0; idx < numberOfRows; idx++ {
		columnsInRow := numberOfColumns
		if len(images)%numberOfRows > 0 && (numberOfRows-idx)*numberOfColumns < len(images)-numberOfColumnsAdded {
			columnsInRow++
		}

		if columnsInRow > maxNumberOfColumns {
			maxNumberOfColumns = columnsInRow
		}

		imagesMatrix[idx] = images[numberOfColumnsAdded : numberOfColumnsAdded+columnsInRow]
		numberOfColumnsAdded += columnsInRow
	}

	maxWidth := uint(0)
	imagesSize := make([][]Size, numberOfRows)
	for row := 0; row < numberOfRows; row++ {
		imagesSize[row] = make([]Size, len(imagesMatrix[row]))

		calculatedWidth := math.Floor(float64(desiredWidth) / float64(len(imagesMatrix[row])))

		rowWidth := uint(0)
		rowHeight := uint(0)
		for col := 0; col < len(imagesMatrix[row]); col++ {
			originalWidth := float64(Width(imagesMatrix[row][col]))
			originalHeight := float64(Height(imagesMatrix[row][col]))
			resizeFactor := calculatedWidth / originalWidth

			w := uint(originalWidth * resizeFactor)
			h := uint(originalHeight * resizeFactor)
			imagesSize[row][col] = Size{w, h}

			if shape == RectangleShape {
				rowWidth += w
			} else {
				rowWidth += uint(math.Min(float64(w), float64(h)) * CircleDiameter)
			}
			rowHeight += h

		}

		if rowWidth > maxWidth {
			maxWidth = rowWidth
		}
	}

	maxHeight := uint(0)
	for col := 0; col < maxNumberOfColumns; col++ {
		colHeight := uint(0)
		for row := 0; row < numberOfRows; row++ {
			if len(imagesSize[row]) > col {
				if shape == RectangleShape {
					colHeight += imagesSize[row][col].height
				} else {
					colHeight += uint(math.Min(float64(imagesSize[row][col].height), float64(imagesSize[row][col].width)) * CircleDiameter)
				}
			}
		}

		if colHeight > maxHeight {
			maxHeight = colHeight
		}
	}

	// output := drawImagesOnBackground(numberOfRows, shape, desiredWidth, maxWidth, maxHeight, maxNumberOfColumns, imagesMatrix)
	output := drawImagesOnBackgroundInParallel(numberOfRows, shape, maxWidth, maxHeight, maxNumberOfColumns, imagesMatrix, desiredWidth)

	return output
}

func calculateImageStartingPointAndSize(img image.Image, imagesMatrix [][]image.Image, padding int, desiredWidth int, shape ImageShape) (ImagePositionAndSize, error) {
	sp_y := padding
	for row := range imagesMatrix {
		sp_x := padding
		calculatedColumnWidth := math.Floor(float64(desiredWidth) / float64(len(imagesMatrix[row])))
		rowHeight := 0

		for col := range imagesMatrix[row] {
			originalWidth := float64(Width(imagesMatrix[row][col]))
			originalHeight := float64(Height(imagesMatrix[row][col]))
			resizeFactor := calculatedColumnWidth / originalWidth

			w := uint(originalWidth * resizeFactor)
			h := uint(originalHeight * resizeFactor)

			if shape == CircleShape {
				w = uint(math.Min(float64(w), float64(h)) * CircleDiameter)
				h = w
			}

			if imagesMatrix[row][col] == img {
				return ImagePositionAndSize{image.Point{sp_x, sp_y}, Size{w, h}}, nil
			} else {
				sp_x += int(w) + padding
			}

			if int(h) > rowHeight {
				rowHeight = int(h)
			}
		}

		sp_y += rowHeight + padding
	}

	return ImagePositionAndSize{image.Point{-1, -1}, Size{0, 0}}, errors.New("Image not found in matrix")
}

func drawSingleImageOnBackground(img image.Image, imagesMatrix [][]image.Image, padding int, shape ImageShape, desiredWidth int, background *MyImage) {
	imageDetails, _ := calculateImageStartingPointAndSize(img, imagesMatrix, padding, desiredWidth, shape)
	sp := imageDetails.sp
	size := imageDetails.size

	if shape == RectangleShape {
		background.drawRaw(img, sp, size.width, size.height)
	} else {
		background.drawInCircle(img, sp, size.width, size.height, int(size.width))
	}
}

func drawImagesOnBackgroundInParallel(numberOfRows int, shape ImageShape, maxWidth uint, maxHeight uint, maxNumberOfColumns int, imagesMatrix [][]image.Image, desiredWidth int) *MyImage {
	padding := 1

	if shape == CircleShape {
		padding = 20
	}

	rectangleEnd := image.Point{int(maxWidth) + (maxNumberOfColumns-1)*padding + 2*padding, int(maxHeight) + (numberOfRows-1)*padding + 2*padding}

	output := MyImage{image.NewRGBA(image.Rectangle{image.ZP, rectangleEnd})}

	for r := range imagesMatrix {
		for c := range imagesMatrix[r] {
			go drawSingleImageOnBackground(imagesMatrix[r][c], imagesMatrix, padding, shape, desiredWidth, &output)
		}
	}

	return &output
}

func drawImagesOnBackground(numberOfRows int, shape ImageShape, desiredWidth int, maxWidth uint, maxHeight uint, maxNumberOfColumns int, imagesMatrix [][]image.Image) *MyImage {
	padding := RectanglePadding

	if shape == CircleShape {
		padding = CirclePadding
	}

	rectangleEnd := image.Point{int(maxWidth) + (maxNumberOfColumns-1)*padding + 2*padding, int(maxHeight) + (numberOfRows-1)*padding + 2*padding}

	output := MyImage{image.NewRGBA(image.Rectangle{image.ZP, rectangleEnd})}

	sp_x, sp_y := 0, 0
	for row := 0; row < numberOfRows; row++ {
		rowHeight := uint(0)

		calculatedWidth := math.Floor(float64(desiredWidth) / float64(len(imagesMatrix[row])))
		for col := 0; col < len(imagesMatrix[row]); col++ {
			originalWidth := float64(Width(imagesMatrix[row][col]))
			originalHeight := float64(Height(imagesMatrix[row][col]))
			resizeFactor := calculatedWidth / originalWidth

			w := uint(originalWidth * resizeFactor)
			h := uint(originalHeight * resizeFactor)

			if col == 0 {
				sp_x = padding
			}

			if row == 0 {
				sp_y = padding
			}

			sp := image.Point{sp_x, sp_y}

			if shape == RectangleShape {
				output.drawRaw(imagesMatrix[row][col], sp, w, h)
			} else {
				w = uint(math.Min(float64(w), float64(h)) * CircleDiameter)
				h = w

				output.drawInCircle(imagesMatrix[row][col], sp, w, h, int(w))
			}

			sp_x += int(w) + padding

			if h > rowHeight {
				rowHeight = h
			}

		}

		sp_x = 0
		sp_y += int(rowHeight) + padding

	}

	return &output
}

// imagecollager will make a collage of images by combining them onto black background
// Script parameters are:
// 1. image shape - share for each inner image inside background - 'Rectangle' or 'Circle'
// 2. number of rows in which images are displayed
// 3. path to the directory where images are stored on file system

func loadImage(path string, info os.FileInfo, images *[]image.Image) {
	if !info.IsDir() {
		fimg, _ := os.Open(path)
		defer fimg.Close()
		img, _, imageError := image.Decode(fimg)

		if imageError == nil {
			*images = append(*images, img)
		}
	}
}

func loadImageChannel(path string, info os.FileInfo, e error, images chan image.Image, errors chan error) {
	if e != nil {
		errors <- e
		return
	}

	if !info.IsDir() {
		fimg, _ := os.Open(path)
		defer fimg.Close()
		img, _, imageError := image.Decode(fimg)

		if imageError == nil {
			images <- img
		} else {
			errors <- imageError
		}
	}
}

func countFiles(dirPath string) (int, error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return 0, err
	}

	counter := 0
	for _, file := range files {
		if !file.IsDir() {
			counter++
		}
	}

	return counter, nil
}

func loadImagesChannel(dirName string, images chan image.Image, quit chan int, errors chan error) {
	err := filepath.Walk(dirName, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			errors <- e
		}

		if !info.IsDir() {
			fimg, _ := os.Open(path)
			defer fimg.Close()
			img, _, imageError := image.Decode(fimg)
			if imageError == nil {
				images <- img
			}
		}
		return nil
	})
	if err != nil {
		errors <- err
	} else {
		quit <- 1
	}
}

func main() {
	if len(os.Args) != 6 {
		log.Fatal("Invalid script call. Should be in format `go run imagecollager.go <Rectangle|Circle> <number of rows> <width> <height>")
	} else {
		imageShape := ImageShape(os.Args[1])
		numberOfRows, errNr := strconv.Atoi(os.Args[2])
		desiredWidth, errDw := strconv.Atoi(os.Args[3])
		desiredHeight, errDh := strconv.Atoi(os.Args[4])

		if errNr == nil && errDw == nil && errDh == nil && (imageShape == RectangleShape || imageShape == CircleShape) {
			readingImagesStart := time.Now()
			var images []image.Image
			dirName := os.Args[5]

			imagesChannel := make(chan image.Image)
			errChannel := make(chan error)

			imagesCount, _ := countFiles(dirName)

			_ = filepath.Walk(dirName, func(path string, info os.FileInfo, e error) error {
				go loadImageChannel(path, info, e, imagesChannel, errChannel)
				return nil
			})

			for {
				select {
				case img := <-imagesChannel:
					images = append(images, img)

					if len(images) == imagesCount {
						readingImagesDuration := time.Since(readingImagesStart)
						log.Print(strconv.Itoa(len(images)) + "Images read in " + readingImagesDuration.String())

						makingCollageStart := time.Now()

						output := makeImageCollage(desiredWidth, desiredHeight, numberOfRows, imageShape, images...)

						makingCollageDuration := time.Since(makingCollageStart)

						log.Print("Making image collage took " + makingCollageDuration.String())

						imview.Show(output.value)
					}
				case <-errChannel:
					log.Fatal("Specified directory with images inside does not exists")
				}
			}
		} else {
			log.Fatal("No shape or number of rows defined")
		}
	}

}
