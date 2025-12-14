# Viewer Application Subtitle System Know-How

## Overview

The fuba_recorder viewer application supports two types of subtitles:
- **VTT (WebVTT)** - Text-based subtitles extracted during the encoding process
- **ASS (Advanced SubStation Alpha)** - Rich formatted subtitles with positioning and styling

## VTT Subtitle Implementation

### File Naming Convention
VTT files must have the same base name as their video files:
- Video: `example_episode_01.mp4`
- Subtitle: `example_episode_01.vtt`

### Browser Compatibility
VTT subtitles use native HTML5 `<track>` element support, ensuring compatibility across all modern browsers without additional libraries.

## ASS Subtitle Implementation

### Key Challenges and Solutions

#### 1. Font Handling
**Problem**: ASS subtitles specify exact fonts that must match for proper rendering and positioning.

**Solution**: 
- Uses WLMaru2004Emoji font as webfont
- Font file served from `/public/wlmaru2004emoji.ttf`
- Loaded via CSS `@font-face` declaration

#### 2. Character Width Calculation
**Problem**: ASS positioning requires precise character width measurements, which vary between fonts.

**Solution**:
- Canvas API measures actual rendered character widths
- Measurements cached for performance
- Handles both half-width and full-width characters correctly

#### 3. Positioning System
**Problem**: ASS uses complex positioning with margins, alignment points, and rotation.

**Solution**:
- Converts ASS position codes (1-9) to CSS alignment
- Calculates exact pixel positions based on video dimensions
- Applies margins and offsets accurately

### ASS Rendering Pipeline

1. **Parse ASS File**
   - Extract styles, dialogue lines, and metadata
   - Parse timing codes and effects

2. **Process Each Subtitle Event**
   - Apply style inheritance (named style → inline overrides)
   - Parse override tags (`{\pos}`, `{\fs}`, etc.)
   - Calculate display timing

3. **Render to DOM**
   - Create positioned `<div>` elements
   - Apply calculated CSS properties
   - Handle line breaks and positioning

4. **Synchronize with Video**
   - Show/hide subtitles based on video currentTime
   - Update on timeupdate events
   - Handle seeking properly

## Technical Implementation Details

### Key Components

#### `SubtitleRenderer.svelte`
Main component that manages subtitle display:
- Loads subtitle files via fetch
- Parses ASS format
- Renders subtitles synchronized with video playback

#### Font Loading Strategy
```javascript
// Ensure font is loaded before rendering
const fontFace = new FontFace('WadaLabMaruGo2004Emoji', 'url(/wlmaru2004emoji.ttf)');
await fontFace.load();
document.fonts.add(fontFace);
```

#### Character Width Measurement
```javascript
// Measure character width using canvas
const canvas = document.createElement('canvas');
const ctx = canvas.getContext('2d');
ctx.font = `${fontSize}px ${fontFamily}`;
const metrics = ctx.measureText(character);
```

### Performance Optimizations

1. **Caching**
   - Character width measurements cached in Map
   - Parsed subtitle data cached
   - Style calculations cached per event

2. **Efficient Updates**
   - Only updates visible subtitles
   - Uses binary search for finding current subtitle
   - Minimal DOM manipulation

3. **Memory Management**
   - Cleans up off-screen subtitle elements
   - Releases parsed data when switching videos

## Common Issues and Solutions

### Issue: Subtitles Not Displaying
**Causes**:
- Missing subtitle file
- Incorrect file naming
- Font not loaded

**Debug Steps**:
1. Check network tab for 404 errors
2. Verify file names match exactly
3. Check console for font loading errors

### Issue: Incorrect Positioning
**Causes**:
- Font metrics mismatch
- Video dimension changes
- Missing ASS position data

**Solutions**:
- Ensure correct font is loaded
- Recalculate positions on video resize
- Fallback to default positions

### Issue: Performance Problems
**Symptoms**:
- Stuttering during playback
- High CPU usage

**Optimizations**:
- Reduce subtitle update frequency
- Pre-calculate positions
- Use CSS transforms instead of position changes

## ASS Format Support

### Supported Features
- Basic positioning (\pos, \move)
- Font styling (\fs, \fn, \b, \i)
- Colors (\c, \3c)
- Transparency (\alpha)
- Line alignment (\an)
- Margins (\marginl, \marginr, \margint)

### Unsupported Features
- Karaoke effects (\k, \kf)
- Complex animations
- Drawing commands
- 3D rotations

## Development Tips

### Testing Subtitles
1. Use test videos with known subtitle timings
2. Test with both VTT and ASS formats
3. Verify on different screen sizes
4. Check performance with subtitle-heavy scenes

### Debugging Tools
- Browser DevTools for DOM inspection
- Performance profiler for optimization
- Network tab for file loading issues
- Console logs for parsing errors

### Adding New Features
When extending subtitle support:
1. Start with ASS format specification
2. Implement parsing first
3. Add rendering with fallbacks
4. Test with real-world subtitle files
5. Profile for performance impacts

## Future Improvements

### Potential Enhancements
1. **Subtitle Style Customization**
   - User-adjustable font size
   - Color scheme options
   - Background opacity control

2. **Advanced Format Support**
   - SRT format support
   - PGS subtitle extraction
   - Multi-language tracks

3. **Performance Optimizations**
   - WebGL rendering for complex effects
   - Worker thread parsing
   - Subtitle pre-rendering

### Architectural Considerations
- Consider subtitle rendering service
- Evaluate WebAssembly for parsing
- Investigate GPU acceleration options

## References

- [ASS Subtitle Format Specification](http://www.tcax.org/docs/ass-specs.htm)
- [WebVTT W3C Specification](https://www.w3.org/TR/webvtt1/)
- [SubtitleOctopus](https://github.com/Dador/JavascriptSubtitlesOctopus) - Alternative ASS renderer (for reference)
