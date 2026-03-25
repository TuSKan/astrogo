package render

import (
	"fmt"
	"image/color"
	"path/filepath"
	"runtime"
	"time"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/gogpu/gg"
)

type ObservationPoint struct {
	Time      time.Time
	TargetAlt float64 // In degrees
	SunAlt    float64 // In degrees
}

type RenderConfig struct {
	Width, Height      int
	DayColor           color.Color
	TwilightColor      color.Color
	NightColor         color.Color
	TargetCurveColor   color.Color
	LimitLineColor     color.Color
	TextColor          color.Color
	UsableLimitDegrees float64 // e.g., 30.0
	Workspace          string
}

// Renderer acts as a configurable container structuring output pipelines.
type Renderer struct {
	config RenderConfig
}

// NewRenderer mounts standard graphical configurations into structural object tracking formats gracefully.
func NewRenderer(cfg RenderConfig) *Renderer {
	return &Renderer{config: cfg}
}

// DefaultConfig returns a highly readable, dark-themed aesthetic
// tailored for astronomical planning charts.
func DefaultConfig() RenderConfig {
	_, filename, _, _ := runtime.Caller(1)
	return RenderConfig{
		Width:  1200,
		Height: 600,

		// Background Shading
		DayColor:      color.RGBA{R: 74, G: 105, B: 132, A: 255}, // Muted daylight blue
		TwilightColor: color.RGBA{R: 26, G: 42, B: 64, A: 255},   // Deep navy astronomical twilight
		NightColor:    color.RGBA{R: 10, G: 14, B: 23, A: 255},   // True pitch dark

		// Data Elements
		TargetCurveColor: color.RGBA{R: 255, G: 215, B: 0, A: 255},   // Bright gold for visibility
		LimitLineColor:   color.RGBA{R: 255, G: 69, B: 0, A: 200},    // Dashed orange/red (30 degrees)
		TextColor:        color.RGBA{R: 224, G: 224, B: 224, A: 255}, // Crisp off-white

		UsableLimitDegrees: 30.0,

		Workspace: filepath.Dir(filename),
	}
}

// DrawFOV generates a visual representation of the target framed strictly against mathematical FOV boundaries properly.
func (r *Renderer) DrawFOV(target catalog.DeepSkyTarget, fovXDeg, fovYDeg float64, outputPath string) error {
	canvasWidth := float64(r.config.Width)
	canvasHeight := float64(r.config.Height)

	// Create context wrapping specified resolution constraints natively
	dc := gg.NewContext(int(canvasWidth), int(canvasHeight))

	// 1. Background Fill explicitly inheriting structural user dimensions universally
	dc.SetColor(r.config.NightColor)
	dc.Clear()

	// 2. Establish Math Engine scale
	// The full 1200px width bounds the exact fovXDeg structurally.
	pixelScale := canvasWidth / fovXDeg

	// 3. Draw Target Object
	// Target axes exist intrinsically in arcminutes, transforming directly to degrees and scaling natively into pixels.
	targDegX := target.MajorAxis / 60.0
	targDegY := target.MinorAxis / 60.0

	// Ellipses require Radii (width/2) internally
	rx := (targDegX * pixelScale) / 2.0
	ry := (targDegY * pixelScale) / 2.0

	centerX := canvasWidth / 2.0
	centerY := canvasHeight / 2.0

	dc.DrawEllipse(centerX, centerY, rx, ry)
	dc.SetColor(r.config.TargetCurveColor) // Bound securely utilizing structural pipeline target colors natively
	dc.Fill()

	// 4. Draw Sensor Frame explicitly tracking identical mathematical borders natively
	sensorWidthPx := fovXDeg * pixelScale
	sensorHeightPx := fovYDeg * pixelScale

	dc.DrawRectangle(
		centerX-(sensorWidthPx/2.0),
		centerY-(sensorHeightPx/2.0),
		sensorWidthPx,
		sensorHeightPx,
	)

	dc.SetColor(r.config.LimitLineColor) // Mapped directly bounding strict framing natively
	dc.SetLineWidth(2.0)
	dc.Stroke()

	// 5. Draw Info Text
	dc.SetColor(r.config.TextColor)
	dc.DrawString(fmt.Sprintf("Target: %s", target.ID), 20, 30)
	dc.DrawString(fmt.Sprintf("FOV: %.2f° x %.2f°", fovXDeg, fovYDeg), 20, 50)

	// Format strings for common names if available
	if target.CommonNames != "" {
		dc.DrawString(fmt.Sprintf("Aliases: %s", target.CommonNames), 20, 70)
	}

	return dc.SavePNG(filepath.Join(r.config.Workspace, outputPath))
}

// DrawTransitCurve plots altitude natively against a timeline capturing advanced mathematical mappings cleanly.
func (r *Renderer) DrawTransitCurve(points []ObservationPoint, outputPath string) error {
	if len(points) == 0 {
		return fmt.Errorf("insufficient altitude mappings to evaluate curves structurally")
	}

	dc := gg.NewContext(r.config.Width, r.config.Height)

	// Determine native canvas scaling evaluating strict boundaries directly natively
	yMax := 50.0
	yMin := float64(r.config.Height) - 50.0

	getY := func(altDeg float64) float64 {
		return yMin - ((altDeg / 90.0) * (yMin - yMax))
	}

	xSpacing := float64(r.config.Width) / float64(len(points)-1)
	if len(points) == 1 {
		xSpacing = float64(r.config.Width) / 2.0
	}

	// Layer 1: Background Shading natively tracing Astronomical Twilight layouts inherently
	for i, pt := range points {
		var fill color.Color
		if pt.SunAlt > 0 {
			fill = r.config.DayColor
		} else if pt.SunAlt >= -18 {
			fill = r.config.TwilightColor
		} else {
			fill = r.config.NightColor
		}

		dc.SetColor(fill)

		xCenter := float64(i) * xSpacing
		x0 := xCenter - (xSpacing / 2.0)

		// Fill bounds wrapping exact margins seamlessly without graphical clipping mathematically
		width := xSpacing
		if i == 0 {
			x0 = 0
			width = xSpacing / 2.0
		} else if i == len(points)-1 {
			width = xSpacing / 2.0
		}

		dc.DrawRectangle(x0, 0, width, float64(r.config.Height))
		dc.Fill()
	}

	// Layer 2: The Usable Line & Horizon limits tracking native mathematical boundaries cleanly
	usableY := getY(r.config.UsableLimitDegrees)

	dc.SetColor(r.config.LimitLineColor)
	dc.SetLineWidth(1.0)
	dc.SetDash(5, 5) // Native dashed limits intersecting geometry purely efficiently.
	dc.DrawLine(0, usableY, float64(r.config.Width), usableY)
	dc.Stroke()

	// Solid horizon evaluating 0 height natively
	horizonY := getY(0.0)
	dc.SetDash() // Reset dash
	dc.SetLineWidth(1.5)
	dc.DrawLine(0, horizonY, float64(r.config.Width), horizonY)
	dc.Stroke()

	// Layer 3: The Target Curve connecting smooth geometries tracing Topocentric positions natively
	dc.SetColor(r.config.TargetCurveColor)
	dc.SetLineWidth(3.0)

	for i, pt := range points {
		x := float64(i) * xSpacing
		y := getY(pt.TargetAlt)

		if y < yMax {
			y = yMax
		} // Clamping mathematically isolating bounds extending off-canvas intrinsically

		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.Stroke()

	// Layer 4: Axis Labels
	dc.SetColor(r.config.TextColor)

	// Y-Axis string representations evaluating bounds cleanly
	for _, deg := range []float64{0, 30, 60, 90} {
		y := getY(deg)
		dc.DrawStringAnchored(fmt.Sprintf("%.0f°", deg), 10, y, 0, 0.5)
	}

	// X-Axis temporal tracking inherently plotting boundaries directly across indices efficiently
	for i, pt := range points {
		if pt.Time.Minute() == 0 || i == 0 || i == len(points)-1 {
			x := float64(i) * xSpacing
			dc.DrawStringAnchored(pt.Time.Format("15:04"), x, float64(r.config.Height)-10, 0.5, 1)
		}
	}

	return dc.SavePNG(filepath.Join(r.config.Workspace, outputPath))
}
