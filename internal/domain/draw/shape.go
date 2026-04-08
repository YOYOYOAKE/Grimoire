package draw

type Shape string

const (
	ShapeSmallPortrait  Shape = "small-portrait"
	ShapeSmallLandscape Shape = "small-landscape"
	ShapeSmallSquare    Shape = "small-square"
	ShapePortrait       Shape = "portrait"
	ShapeLandscape      Shape = "landscape"
	ShapeSquare         Shape = "square"
	ShapeLargePortrait  Shape = "large-portrait"
	ShapeLargeLandscape Shape = "large-landscape"
	ShapeLargeSquare    Shape = "large-square"
)

func (s Shape) Valid() bool {
	switch s {
	case ShapeSmallPortrait, ShapeSmallLandscape, ShapeSmallSquare,
		ShapePortrait, ShapeLandscape, ShapeSquare,
		ShapeLargePortrait, ShapeLargeLandscape, ShapeLargeSquare:
		return true
	default:
		return false
	}
}

func (s Shape) Label() string {
	switch s {
	case ShapeSmallPortrait:
		return "Small Portrait (512x768)"
	case ShapeSmallLandscape:
		return "Small Landscape (768x512)"
	case ShapeSmallSquare:
		return "Small Square (640x640)"
	case ShapePortrait:
		return "Normal Portrait (832x1216)"
	case ShapeLandscape:
		return "Normal Landscape (1216x832)"
	case ShapeSquare:
		return "Normal Square (1024x1024)"
	case ShapeLargePortrait:
		return "Large Portrait (1024x1536)"
	case ShapeLargeLandscape:
		return "Large Landscape (1536x1024)"
	case ShapeLargeSquare:
		return "Large Square (1472x1472)"
	default:
		return string(s)
	}
}
