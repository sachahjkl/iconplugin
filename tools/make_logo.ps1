# Draws the iconplugin logo (512x512 PNG): an indigo rounded square with a soft vertical gradient and a darker border,
# carrying a 2x2 grid of white icon glyphs (square, circle, triangle, sparkle). Holes show the background through.
#   powershell -File tools/make_logo.ps1 [-Out .project/image.png]
param([string]$Out = (Join-Path (Split-Path $PSScriptRoot -Parent) '.project\image.png'))
Add-Type -AssemblyName System.Drawing
$S = 4; $N = 512 * $S   # supersampled, then downscaled

function Hex($v) { return [Drawing.Color]::FromArgb(255, ($v -shr 16) -band 255, ($v -shr 8) -band 255, $v -band 255) }
function RoundRect($x, $y, $w, $h, $r) {
  $p = New-Object Drawing.Drawing2D.GraphicsPath; $d = 2 * $r
  $p.AddArc($x, $y, $d, $d, 180, 90); $p.AddArc($x + $w - $d, $y, $d, $d, 270, 90)
  $p.AddArc($x + $w - $d, $y + $h - $d, $d, $d, 0, 90); $p.AddArc($x, $y + $h - $d, $d, $d, 90, 90); $p.CloseFigure(); return $p
}
function Pt($x, $y) { return New-Object Drawing.PointF ([single]($x * $S)), ([single]($y * $S)) }
function Poly($pts) { return [Drawing.PointF[]]($pts | ForEach-Object { Pt $_[0] $_[1] }) }

$b = New-Object Drawing.Bitmap $N, $N
$g = [Drawing.Graphics]::FromImage($b); $g.SmoothingMode = 'AntiAlias'; $g.Clear([Drawing.Color]::Transparent)

# indigo body: lighter at the top, deeper at the bottom
$fill = New-Object Drawing.Drawing2D.LinearGradientBrush (Pt 0 56), (Pt 0 456), (Hex 0x7a5af0), (Hex 0x3b1fa8)
$g.FillPath($fill, (RoundRect (56*$S) (56*$S) (400*$S) (400*$S) (100*$S)))

# darker border, drawn just inside the edge (also a gradient, so it stays darker than the body everywhere)
$bw = 10
$edge = New-Object Drawing.Drawing2D.LinearGradientBrush (Pt 0 56), (Pt 0 456), (Hex 0x5a3dc8), (Hex 0x2a1480)
$pen = New-Object Drawing.Pen $edge, ([single]($bw * $S))
$g.DrawPath($pen, (RoundRect ((56 + $bw/2)*$S) ((56 + $bw/2)*$S) ((400 - $bw)*$S) ((400 - $bw)*$S) ((100 - $bw/2)*$S)))

# a faint highlight along the top inner edge
$hl = New-Object Drawing.Pen ([Drawing.Color]::FromArgb(40, 255, 255, 255)), ([single](2 * $S))
$g.DrawPath($hl, (RoundRect ((56 + $bw + 1)*$S) ((56 + $bw + 1)*$S) ((400 - 2*$bw - 2)*$S) ((400 - 2*$bw - 2)*$S) ((100 - $bw)*$S)))

$white = New-Object Drawing.SolidBrush ([Drawing.Color]::White)
$wpen = New-Object Drawing.Pen ([Drawing.Color]::White), ([single](18 * $S)); $wpen.LineJoin = 'Round'
$cx1 = 188; $cx2 = 324; $cy1 = 188; $cy2 = 324

# top-left: rounded square with a hole
$g.FillPath($white, (RoundRect (($cx1-50)*$S) (($cy1-50)*$S) (100*$S) (100*$S) (26*$S)))
$g.FillPath($fill, (RoundRect (($cx1-22)*$S) (($cy1-22)*$S) (44*$S) (44*$S) (10*$S)))

# top-right: ring
$g.FillEllipse($white, [single](($cx2-52)*$S), [single](($cy1-52)*$S), [single](104*$S), [single](104*$S))
$g.FillEllipse($fill, [single](($cx2-24)*$S), [single](($cy1-24)*$S), [single](48*$S), [single](48*$S))

# bottom-left: triangle with rounded corners
$tri = Poly @(@($cx1, ($cy2-44)), @(($cx1+46), ($cy2+36)), @(($cx1-46), ($cy2+36)))
$g.FillPolygon($white, $tri); $g.DrawPolygon($wpen, $tri)

# bottom-right: four-point sparkle
$sp = @()
for ($i = 0; $i -lt 8; $i++) {
  $a = [Math]::PI / 4 * $i - [Math]::PI / 2; $r = if ($i % 2 -eq 0) { 60 } else { 20 }
  $sp += ,@(($cx2 + $r * [Math]::Cos($a)), ($cy2 + $r * [Math]::Sin($a)))
}
$spp = Poly $sp
$g.FillPolygon($white, $spp); $g.DrawPolygon($wpen, $spp)

$o = New-Object Drawing.Bitmap 512, 512
$g2 = [Drawing.Graphics]::FromImage($o); $g2.InterpolationMode = 'HighQualityBicubic'; $g2.PixelOffsetMode = 'HighQuality'
$g2.DrawImage($b, 0, 0, 512, 512)
$o.Save($Out, [Drawing.Imaging.ImageFormat]::Png)
"wrote $Out"
