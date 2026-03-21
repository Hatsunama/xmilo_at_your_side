# xMilo Art Asset Specification — v11

## Character Reference: Milo the Wizard Duck

### Visual Identity (based on confirmed reference images)

**Body**
- Species: Anthropomorphic duck standing fully upright on two legs
- Skin/feather color: Muted lime-green (#7DBF6B range) — not yellow, not bright green.
  The exact shade is a desaturated warm green, slightly textured like feathers.
- Bill/beak: Orange (#E87E30 range), rounded duck bill, slightly bulbous at tip
- Feet: Orange-yellow webbed duck feet, same color family as bill
- Hands: Green, three-fingered (duck-like), stubby fingers
- Body shape: Rounded, slightly pudgy. Not tall and thin. Compact and charismatic.
- Expression baseline: Half-lidded eyes, slightly skeptical/tired/knowing.
  NOT angry. NOT happy. The vibe is "competent and mildly exasperated."
  Think: a professor who has seen everything before.

**Robe**
- Color: Dark navy blue leaning toward deep purple (#1A1850 to #231B6B range)
- Stars: Gold/tan 5-pointed stars scattered across the entire robe surface.
  Stars vary slightly in size. Not perfectly uniform. Hand-drawn feel.
  Star color: warm gold (#C8A84B range), not bright yellow.
- Cuffs: Slightly lighter band at the wrist, same gold/tan color
- Length: Falls to mid-thigh on a standing duck. Covers the body fully.
  Flares slightly at the bottom.
- Style: Wide wizard robe, large sleeves. Not form-fitting.

**Hat**
- Style: Tall pointed wizard hat, slightly floppy/bent at the very tip
- Color: Same dark navy/purple as the robe — they match
- Stars: Same gold stars as the robe, scattered across the hat
- Brim: Slightly wider than the head base
- Tilt: Hat sits slightly forward on the head, tip leans in one direction

**Accessories**
- Necklace: Silver chain visible at the collar, with a small geometric pendant.
  The pendant shape is approximately a cross with triangular points (like a
  compass rose or geometric star). Silver/pewter colored.
- Glasses (optional — some reference images show them): Round wire-frame
  reading glasses, copper/brass colored. Only present in reading/studying poses.

**Art Style**
- 2D cartoon/comic illustration
- Thick black outlines (#1A1A1A, 2-3px equivalent)
- Flat colors with minimal shading — cel-shading style
- Warm lighting (candle/amber ambient light affects all scenes)
- NOT photorealistic. NOT pixel art. NOT chibi.
  Target aesthetic: high-quality mobile game character art.
  Reference comparison: the art style in the uploaded reference images exactly.

---

## Sprite Sheet Specification for castle-go

All sprites are used by `castle-go/internal/assets/assets.go`.

### Milo Sprite Sheets

**File path pattern:** `castle-go/internal/assets/sprites/milo/{state}_{facing}.png`

**Sheet dimensions:** 512 × 80 pixels
- 8 frames per sheet, arranged horizontally left to right
- Each frame: 64 × 80 pixels
- No padding between frames
- Transparent background (PNG with alpha)

**Viewing angle:** Isometric 2:1, camera pointing from upper-right (southeast).
This is the same angle used in The Sims 1. The character is viewed from
approximately 45° azimuth and 30° elevation. For sprite purposes, this means:

- Facing "s" (south): Milo is walking toward the viewer, slightly right. Full front.
- Facing "n" (north): Milo is walking away from viewer, slightly left. Back view.
- Facing "e" (east): Milo is walking right and toward camera. 3/4 right profile.
- Facing "w" (west): Milo is walking left and away from camera. 3/4 left profile.

**Anchor point:** Bottom-center of each frame is Milo's feet/ground contact point.
The renderer places this point at the tile position. Leave 4-6px of space at
the bottom of each frame for the feet.

---

### Animation States (9 states × 4 facings = 36 PNG files)

**idle_n, idle_s, idle_e, idle_w**
Milo standing still with a subtle breathing cycle.
Frame sequence: slight body bob up/down, hat sways very slightly.
Arms at sides or crossed (crossed arms preferred — matches reference image 4).
8 frames covering one full breath cycle.
Expression: default half-lidded, slightly skeptical.

**walking_n, walking_s, walking_e, walking_w**
Milo walking. Standard bipedal walk cycle.
Robe sways with movement. Hat bobs slightly.
Arms swing naturally at sides (robe sleeves hide most of the arm motion).
8 frames covering one full stride cycle (left foot → right foot).

**talking_s** (only south facing needed — Milo talks to user facing camera)
Bill opens and closes in speech pattern.
Slight head bob. One hand raised partially (gesturing).
8 frames: 4 with bill slightly open, 4 with bill more open. Cycles.
Optional: small stars or sparkles near the bill to indicate speech.

**thinking_s** (south only — facing user when thinking about a task)
One hand raised to the side of the head (the "thinking" pose).
Eyes look upward or sideways. Bill slightly scrunched.
8 frames: very subtle shift — mostly a held pose with minor eye movement.

**sleeping_s** (south — for inactivity state, at main_hall_center)
Milo sitting or slumping slightly. Eyes fully closed.
Small ZZZ letters float upward (can be on the sprite or handled as overlay).
8 frames: gentle up/down breathing, ZZZ drift slowly upward.
Expression: peaceful, not distressed.

**working_s, working_e** (south + east — general task work pose)
Milo leaning slightly forward, focused. Bill slightly open (concentrating).
One hand forward as if writing or gesturing at something.
8 frames: subtle intensity animation — slight forward lean pulse.

**reading_s, reading_e** (library, spellbook, archive rooms)
Milo hunched slightly over, as if reading a book.
Glasses appear in this state (the round brass reading glasses).
Head tilts slightly down. Bill pointed toward imaginary book surface.
8 frames: occasional page-turn gesture on frame 7-8, then loops from frame 1.

**stirring_e** (cauldron room — east facing, toward the cauldron)
Milo's arm extended forward in a slow circular stirring motion.
Robe sleeve sways with the arm arc.
8 frames covering one full clockwise stirring circle.
Expression: slightly suspicious of the bubbling mixture.

**gazing_s, gazing_e** (crystal orb room)
Milo with both hands slightly raised toward an imaginary orb.
Eyes wide open (the one state where his eyes are fully open).
Slight forward lean. Glow from the orb would be handled as ambient overlay.
8 frames: subtle hand tremor/pulse, eyes may flicker to half-lidded briefly.

---

## Room Background Specification

**File path pattern:** `castle-go/internal/assets/rooms/{roomID}.png`
**Dimensions:** 960 × 720 pixels
**Format:** PNG, no alpha (full opaque background, fills screen)
**Camera angle:** Same 2:1 isometric, fixed camera. All rooms use identical angle.
**Art style:** Matches Milo — thick outlines, cel-shaded, warm candlelight ambient.

### Color palette across all rooms
- Shadows: Deep navy/charcoal (#0D0A1A to #1A1535)
- Stone walls: Cool grey-blue (#3D3D4F)
- Wood surfaces: Warm brown (#5C3D20 to #7A4F2A)
- Candle light: Amber (#FFA840 with soft glow halo)
- Magic effects: Purple (#7B3FA6), teal (#40B8A6), gold (#C8A84B)

---

### main_hall.png
Grand stone hall, the primary space.
- Stone floor with cracked tile pattern, slight diagonal grid visible
- Two tall banner/tapestry stands on left and right walls, gold border
- Throne on a 2-step raised dais at center-back, dark wood, purple cushion
- Lit fireplace on back wall, warm amber glow
- Stone arch doorway implied at front-left (entry)
- Two wall-mounted torch sconces provide warm side lighting
- Ambient mood: welcoming but imposing

### war_room.png
Tactical planning chamber.
- Large rectangular wooden table covered with a parchment map, center of room
- Wall-mounted large map (showing a fantasy continent) on back wall, lit by lamp
- Two flagpoles on either side — dark purple flag with gold star (Milo's sigil)
- Bookshelves to one side with thick tactical tomes
- Dim lighting, only map lamp and one candelabra
- Ambient mood: serious, focused

### library.png
Tall reading room.
- Floor-to-ceiling bookshelves on both visible walls, packed with varied books
- Central reading desk with open book, quill in inkwell, magnifying glass
- Reading lamp (green glass shade, brass body) on the desk
- Rolling ladder on the east shelf
- Small window implied at top-back letting in moonlight (blue tint)
- Warm ambient from the desk lamp, cool from moonlight
- Globe on a stand in one corner
- Ambient mood: scholarly, cozy

### training_room.png
Open martial practice space.
- Stone floor with worn practice mat (dark red/brown), center
- Wooden training dummy (post with two side arms) to the right
- Weapon rack on the back wall: two swords, a staff, crossed axes
- Torch sconces on walls, brighter than other rooms
- Empty space for movement in center
- Ambient mood: purposeful, energetic

### spellbook.png
Intimate study/writing alcove.
- Ornate wooden lectern center-left, large open spellbook on it
  (book has glowing runes/text, faint purple light from pages)
- Tall candelabra (3 candles) to the right
- Scroll rack on the back wall — hanging rolled scrolls
- Small table with ink bottles, quills, wax seal kit
- Crystal paperweight on the lectern
- Low ceiling, cozy and intimate
- Ambient mood: arcane, focused, slightly magical

### cauldron.png
Stone alchemy chamber.
- Large iron cauldron on a stone fire pit, center-left
  (default: NOT bubbling — ambient effect activates the bubble)
- Ingredient shelf on the back wall: jars of colored substances, dried herbs
- Potion rack on the right: 4-6 glass bottles, various colored liquids
- Fire pit glow: deep orange-red beneath the cauldron
- Smoke/steam wisps rising from cauldron opening
- Green-tinted ambient from the potion bottles
- Ambient mood: experimental, slightly ominous

### crystal_orb.png
Circular divination chamber.
- Crystal orb on a tall carved stone plinth, dead center of room
  (default: dim — ambient effect activates the glow and pulse)
- Circular rune inscription on the floor around the plinth
- Star map painted on the ceiling (visible from the angle as the back wall)
- Two candelabras on either side for base lighting
- Deep blue and purple ambient, cool and mysterious
- Ambient mood: mystical, quiet

### baby_dragon.png
Warm den/nest chamber.
- High stone perch (2-3 tiers of rock formation) in the back-right
- Small pile of toys and objects at the base of the perch (balls, bones, books)
- Gem hoard in the back-left corner: small pile of colorful gems/coins
- Warm torch lighting, orange-amber dominant
- Straw/nest material on the floor near the perch base
- The room is slightly rougher/more organic than the others
- NO dragon character — that is a prop sprite placed separately
- Ambient mood: warm, playful, lived-in

### trophy.png
Hall of achievement.
- Trophy case (glass-fronted wooden cabinet) along the back wall
  holding 3-5 trophies of varying sizes
- Victory banners hanging from the walls
- Pedestal in the center-right for a featured trophy
- Plaques on the side walls (text not legible)
- Gold and warm lighting throughout — celebratory
- Ambient mood: proud, bright

### archive.png
Deep record-keeping vault.
- Long rows of scroll shelves receding into the back (creates depth)
- Stone lectern in the center foreground, open archive book
- Floating/hovering memory crystal above the lectern (small, dim blue by default)
- Brass filing cabinets on the right wall
- Cool blue-grey lighting, dusty ambient
- Ceiling is lower than other rooms — intimate, archival
- Ambient mood: quiet, timeless

---

## Prop Sprites

**File path pattern:** `castle-go/internal/assets/sprites/props/{propKey}.png`
**Dimensions:** Variable per prop, anchor at bottom-center
**Format:** PNG with transparency

The prop keys used in rooms.go are:
throne, banner_left, banner_right, fireplace,
strategy_table, map_wall, flag_left, flag_right,
reading_desk, bookshelf_east, bookshelf_north, reading_lamp,
training_dummy, weapon_rack, training_mat,
spellbook_stand, scroll_shelf, inkwell, candle_cluster,
cauldron, ingredient_shelf, fire_pit, potion_rack,
orb_plinth, crystal_orb, star_map, rune_circle,
dragon_perch, dragon, toy_pile, gem_hoard,
trophy_case, trophy_pedestal, victory_banner, achievement_wall,
archive_lectern, archive_shelf_a, archive_shelf_b, memory_crystal

Recommended dimensions per prop category:
- Large furniture (tables, shelves, throne): 96-128 × 128-160 px
- Small props (candles, inkwell, crystal): 32-48 × 48-64 px
- Dragon: 128 × 128 px (it needs presence on the perch)

All props should match the same cel-shaded cartoon style as Milo and the rooms.

---

## Generation Prompt Template (for AI image generators)

Use the following base prompt structure when generating Milo sprite sheets:

```
Anthropomorphic green duck wizard, cartoon 2D game sprite, cel shaded illustration,
thick black outlines, wearing dark navy blue wizard robe covered in gold stars,
tall pointed navy wizard hat with gold stars, orange bill, orange webbed feet,
silver chain necklace with geometric pendant, green feathered skin,
half-lidded skeptical expression, isometric view from upper right at 30 degrees elevation,
warm candle lighting, transparent background, [STATE DESCRIPTION], [FACING DESCRIPTION],
horizontal sprite sheet with 8 frames, 512x80 pixels total, no padding between frames
```

Replace [STATE DESCRIPTION] with the animation state description above.
Replace [FACING DESCRIPTION] with the direction facing.

For rooms, use:
```
Isometric 2.5D game room background, castle interior, cartoon illustration,
cel shaded style, thick black outlines, warm amber candlelight ambient,
[ROOM DESCRIPTION], viewed from upper right at 30 degrees isometric angle,
960x720 pixels, no characters, only furniture and props
```
