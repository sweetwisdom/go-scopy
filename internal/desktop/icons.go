package desktop

import "github.com/vivaui/viva/ui"

// Small application-specific line icons, declared once outside painting.
var iconPhone = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(8, 2), ui.LineTo(16, 2), ui.QuadTo(19, 2, 19, 5), ui.LineTo(19, 19), ui.QuadTo(19, 22, 16, 22), ui.LineTo(8, 22), ui.QuadTo(5, 22, 5, 19), ui.LineTo(5, 5), ui.QuadTo(5, 2, 8, 2), ui.ClosePath(), ui.MoveTo(10, 5), ui.LineTo(14, 5), ui.MoveTo(11, 19), ui.LineTo(13, 19),
}}
var iconTasks = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(9, 5), ui.LineTo(21, 5), ui.MoveTo(9, 12), ui.LineTo(21, 12), ui.MoveTo(9, 19), ui.LineTo(21, 19), ui.MoveTo(2, 5), ui.LineTo(3, 6), ui.LineTo(5, 3), ui.MoveTo(2, 12), ui.LineTo(3, 13), ui.LineTo(5, 10), ui.MoveTo(2, 19), ui.LineTo(3, 20), ui.LineTo(5, 17),
}}
var iconSettings = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(4, 5), ui.LineTo(20, 5), ui.MoveTo(4, 12), ui.LineTo(20, 12), ui.MoveTo(4, 19), ui.LineTo(20, 19), ui.MoveTo(8, 2), ui.LineTo(8, 8), ui.MoveTo(16, 9), ui.LineTo(16, 15), ui.MoveTo(10, 16), ui.LineTo(10, 22),
}}
var iconTheme = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(20, 15), ui.CubicTo(16, 17, 10, 15, 9, 11), ui.CubicTo(8, 8, 9, 5, 11, 3), ui.CubicTo(5, 3, 2, 8, 3, 14), ui.CubicTo(4, 20, 11, 23, 16, 20), ui.CubicTo(18, 19, 19, 17, 20, 15), ui.ClosePath(),
}}
