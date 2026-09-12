"""Build rolle wordmark outlines. Requires fonttools==4.65.0; no runtime fonts.

Usage: python build_wordmark.py FONT_TTF STACK_SVG OUTPUT_DIRECTORY
The unmodified font is MuseoModerno at weight 700. Only the logo's r is custom.
"""
from pathlib import Path
import sys
import xml.etree.ElementTree as ET
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.boundsPen import BoundsPen

FONT, STACK, OUT = Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3])
OUT.mkdir(parents=True, exist_ok=True)
font = instantiateVariableFont(TTFont(FONT), {'wght': 700})
glyphs = font.getGlyphSet()
cmap = font.getBestCmap()

def glyph(char):
    g = glyphs[cmap[ord(char)]]
    pen = SVGPathPen(glyphs)
    bounds = BoundsPen(glyphs)
    g.draw(pen); g.draw(bounds)
    return pen.getCommands(), bounds.bounds

r_path, r_bounds = glyph('r')
scale = 132 / r_bounds[3]
baseline = glyph('l')[1][3] * scale
height = baseline + 6

# Exact icon counter translated by (-264, -282); scale never changes its aspect.
ICON_R = 'M 0 132 L 48 132 L 48 64 Q 48 34 78 34 L 102 34 L 102 0 L 72 0 Q 0 0 0 72 Z'
word = [f'<path id="icon-r" transform="translate(0 {baseline-132:.6f})" d="{ICON_R}"/>']
cursor = 102
gap = 16
for char in 'olle':
    d, (xmin, ymin, xmax, ymax) = glyph(char)
    cursor += gap
    word.append(f'<path transform="translate({cursor-xmin*scale:.6f} {baseline:.6f}) scale({scale:.9f} {-scale:.9f})" d="{d}"/>')
    cursor += (xmax-xmin)*scale
width = cursor
word = ''.join(word)

def svg(width, height, body, title):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{width:.3f}" height="{height:.3f}" viewBox="0 0 {width:.3f} {height:.3f}" role="img" aria-label="{title}"><title>{title}</title>{body}</svg>\n'

for name, ink in [('wordmark-ivory','#F4F0E8'),('wordmark-charcoal','#181A1E'),('wordmark-blue','#244CFF')]:
    (OUT/(name+'.svg')).write_text(svg(width+32,height+32,f'<g transform="translate(16 16)" fill="{ink}">{word}</g>','rolle custom wordmark'))

stack = ET.parse(STACK).getroot()
paths = ''.join(f'<path fill="{p.attrib["fill"]}" d="{p.attrib["d"]}"/>' for p in stack.findall('{http://www.w3.org/2000/svg}path'))
assert len(stack.findall('{http://www.w3.org/2000/svg}path')) == 3
assert 'L 312 414 L 312 346 Q 312 316 342 316 L 366 316 L 366 282 L 336 282 Q 264 282 264 354 L 264 414' in paths, 'Icon r changed; update the matching wordmark path deliberately.'

lockup_width = 352+70+width+64
lockup_height = 380
body = f'<g transform="translate(-48 -66)" >{paths}</g><g transform="translate(454 {(lockup_height-height)/2:.6f})" fill="#F4F0E8">{word}</g>'
(OUT/'lockup-dark.svg').write_text(svg(lockup_width,lockup_height,body,'rolle stack and custom wordmark for dark backgrounds'))
body_light = body.replace('#F4F0E8','#181A1E')
(OUT/'lockup-light.svg').write_text(svg(lockup_width,lockup_height,body_light,'rolle stack and custom wordmark for light backgrounds'))
(OUT/'icon-r.svg').write_text(svg(134,164,f'<path transform="translate(16 16)" fill="#244CFF" d="{ICON_R}"/>','rolle icon r, exact positive shape'))

preview = '<rect width="1200" height="720" fill="#101114"/>'
preview += '<text x="44" y="46" fill="#B4B8C0" font-family="sans-serif" font-size="16" letter-spacing="3">ROLLE / ORIGINAL CIRCUIT</text>'
preview += f'<g transform="translate({(1200-lockup_width)/2:.6f} 96)">{body}</g>'
preview += '<path d="M44 510H1156" stroke="#3F4248"/>'
preview += '<text x="44" y="552" fill="#F4F0E8" font-family="sans-serif" font-size="18">MuseoModerno Bold + the exact icon r</text>'
preview += '<text x="44" y="585" fill="#B4B8C0" font-family="sans-serif" font-size="15">Same curve. Same terminals. Same proportions.</text>'
preview += f'<rect x="716" y="544" width="104" height="116" rx="12" fill="#00CE78"/><path transform="translate(734 556) scale(.67)" fill="#181A1E" d="{ICON_R}"/>'
preview += f'<path transform="translate(932 556) scale(.67)" fill="#F4F0E8" d="{ICON_R}"/>'
preview += '<text x="716" y="688" fill="#B4B8C0" font-family="sans-serif" font-size="13">Icon counter</text><text x="917" y="688" fill="#B4B8C0" font-family="sans-serif" font-size="13">Wordmark letter</text>'
(OUT/'wordmark-preview.svg').write_text(svg(1200,720,preview,'rolle wordmark matched to the icon r'))
print(f'Created outlined wordmark and lockups. Wordmark bounds: {width:.2f} × {height:.2f}. Exact icon r reused.')
