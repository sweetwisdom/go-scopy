package desktop

import "github.com/vivaui/viva/ui"

var iconCamera = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(3, 6), ui.LineTo(8, 6), ui.LineTo(9, 3), ui.LineTo(15, 3), ui.LineTo(16, 6), ui.LineTo(21, 6), ui.LineTo(21, 20), ui.LineTo(3, 20), ui.ClosePath(),
	ui.MoveTo(16, 13), ui.CubicTo(16, 18, 8, 18, 8, 13), ui.CubicTo(8, 8, 16, 8, 16, 13), ui.ClosePath(),
}}

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

var iconScript = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(8, 3), ui.LineTo(19, 3), ui.LineTo(19, 21), ui.LineTo(8, 21), ui.MoveTo(11, 8), ui.LineTo(8, 11), ui.LineTo(11, 14), ui.MoveTo(14, 8), ui.LineTo(17, 11), ui.LineTo(14, 14), ui.MoveTo(4, 7), ui.LineTo(4, 18),
}}

var iconEdit = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(15, 4), ui.LineTo(20, 9), ui.LineTo(8, 21), ui.LineTo(3, 21), ui.LineTo(3, 16), ui.ClosePath(), ui.MoveTo(13, 6), ui.LineTo(18, 11),
}}
var iconDelete = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(3, 6), ui.LineTo(21, 6), ui.MoveTo(9, 6), ui.LineTo(9, 3), ui.LineTo(15, 3), ui.LineTo(15, 6), ui.MoveTo(5, 6), ui.LineTo(6, 21), ui.LineTo(18, 21), ui.LineTo(19, 6), ui.MoveTo(10, 10), ui.LineTo(10, 17), ui.MoveTo(14, 10), ui.LineTo(14, 17),
}}

// Connected/disconnected link glyphs follow the application's existing line icons.
var iconConnect = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(10, 13), ui.CubicTo(8, 11, 8, 8, 10, 6), ui.LineTo(13, 3), ui.CubicTo(18, -1, 25, 6, 21, 11), ui.LineTo(18, 14), ui.CubicTo(16, 16, 13, 16, 11, 14),
	ui.MoveTo(14, 11), ui.CubicTo(16, 13, 16, 16, 14, 18), ui.LineTo(11, 21), ui.CubicTo(6, 25, -1, 18, 3, 13), ui.LineTo(6, 10), ui.CubicTo(8, 8, 11, 8, 13, 10),
}}
var iconDisconnect = ui.Icon{ViewBox: 24, Stroke: 1.7, Cap: ui.CapRound, Join: ui.JoinRound, Commands: []ui.PathCommand{
	ui.MoveTo(15, 7), ui.LineTo(17, 5), ui.CubicTo(21, 1, 25, 7, 21, 11), ui.LineTo(19, 13),
	ui.MoveTo(9, 17), ui.LineTo(7, 19), ui.CubicTo(3, 23, -1, 17, 3, 13), ui.LineTo(5, 11),
	ui.MoveTo(9, 3), ui.LineTo(9, 6), ui.MoveTo(18, 15), ui.LineTo(21, 15), ui.MoveTo(3, 9), ui.LineTo(6, 9), ui.MoveTo(15, 18), ui.LineTo(15, 21), ui.MoveTo(8, 8), ui.LineTo(16, 16),
}}
