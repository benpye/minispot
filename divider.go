package main

import (
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

const (
	DirectionHorizontal = iota
	DirectionVertical
)

type Divider struct {
	x, y, width, height int

	direction int

	backgroundColor  tcell.Color
	borderColor      tcell.Color
	borderAttributes tcell.AttrMask
}

func NewDivider() *Divider {
	return &Divider{
		direction:       DirectionVertical,
		backgroundColor: tview.Styles.PrimitiveBackgroundColor,
		borderColor:     tview.Styles.BorderColor,
	}
}

func (d *Divider) Draw(screen tcell.Screen) {
	if d.width <= 0 || d.height <= 0 {
		return
	}

	def := tcell.StyleDefault

	background := def.Background(d.backgroundColor)
	border := background.Foreground(d.borderColor) | tcell.Style(d.borderAttributes)

	if d.direction == DirectionHorizontal {
		for x := d.x; x < d.x+d.width; x++ {
			screen.SetContent(x, d.y, tview.Borders.Horizontal, nil, border)
		}
	} else if d.direction == DirectionVertical {
		for y := d.y; y < d.y+d.height; y++ {
			screen.SetContent(d.x, y, tview.Borders.Vertical, nil, border)
		}
	}
}

func (d *Divider) SetHorizontal(direction int) *Divider {
	d.direction = direction
	return d
}

func (d *Divider) SetBackgroundColor(color tcell.Color) *Divider {
	d.backgroundColor = color
	return d
}

func (d *Divider) SetBorderColor(color tcell.Color) *Divider {
	d.borderColor = color
	return d
}

func (d *Divider) GetRect() (int, int, int, int) {
	return d.x, d.y, d.width, d.height
}

func (d *Divider) SetRect(x, y, width, height int) {
	d.x = x
	d.y = y
	d.width = width
	d.height = height
}

func (d *Divider) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return nil
}

func (d *Divider) Focus(delegate func(p tview.Primitive)) {
	return
}

func (d *Divider) Blur() {
	return
}

func (d *Divider) HasFocus() bool {
	return false
}

func (d *Divider) GetFocusable() tview.Focusable {
	return d
}
