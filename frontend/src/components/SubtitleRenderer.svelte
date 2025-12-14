<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  
  export let subtitleUrl: string | null = null
  export let videoElement: HTMLVideoElement | null = null
  
  interface AssStyle {
    name: string
    fontname: string
    fontsize: number
    primaryColour: string
    secondaryColour: string
    outlineColour: string
    backColour: string
    bold: number
    italic: number
    underline: number
    strikeOut: number
    scaleX: number
    scaleY: number
    spacing: number
    angle: number
    borderStyle: number
    outline: number
    shadow: number
    alignment: number
    marginL: number
    marginR: number
    marginV: number
  }
  
  interface AssDialogue {
    layer: number
    start: number
    end: number
    style: string
    name: string
    marginL: number
    marginR: number
    marginV: number
    effect: string
    text: string
  }
  
  let subtitles: AssDialogue[] = []
  let styles: Record<string, AssStyle> = {}
  let activeSubtitles: AssDialogue[] = []
  let subtitleContainer: HTMLDivElement
  
  $: if (subtitleUrl) {
    loadSubtitles(subtitleUrl)
  }
  
  $: if (videoElement) {
    videoElement.addEventListener('timeupdate', updateSubtitles)
  }
  
  async function loadSubtitles(url: string) {
    try {
      const response = await fetch(url)
      const text = await response.text()
      parseAssSubtitles(text)
    } catch (error) {
      console.error('Failed to load subtitles:', error)
    }
  }
  
  function parseAssSubtitles(content: string) {
    const lines = content.split('\n')
    let section = ''
    
    subtitles = []
    styles = {}
    
    for (const line of lines) {
      const trimmed = line.trim()
      
      if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
        section = trimmed.slice(1, -1)
        continue
      }
      
      if (section === 'V4+ Styles' && trimmed.startsWith('Style:')) {
        parseStyle(trimmed)
      } else if (section === 'Events' && trimmed.startsWith('Dialogue:')) {
        const dialogue = parseDialogue(trimmed)
        if (dialogue) subtitles.push(dialogue)
      }
    }
    
    subtitles.sort((a, b) => a.start - b.start)
  }
  
  function parseStyle(line: string) {
    const parts = line.slice(7).split(',').map(p => p.trim())
    if (parts.length < 23) return
    
    const style: AssStyle = {
      name: parts[0],
      fontname: parts[1],
      fontsize: parseFloat(parts[2]),
      primaryColour: parts[3],
      secondaryColour: parts[4],
      outlineColour: parts[5],
      backColour: parts[6],
      bold: parseInt(parts[7]),
      italic: parseInt(parts[8]),
      underline: parseInt(parts[9]),
      strikeOut: parseInt(parts[10]),
      scaleX: parseFloat(parts[11]),
      scaleY: parseFloat(parts[12]),
      spacing: parseFloat(parts[13]),
      angle: parseFloat(parts[14]),
      borderStyle: parseInt(parts[15]),
      outline: parseFloat(parts[16]),
      shadow: parseFloat(parts[17]),
      alignment: parseInt(parts[18]),
      marginL: parseInt(parts[19]),
      marginR: parseInt(parts[20]),
      marginV: parseInt(parts[21])
    }
    
    styles[style.name] = style
  }
  
  function parseDialogue(line: string) {
    const parts = line.slice(10).split(',')
    if (parts.length < 10) return null
    
    return {
      layer: parseInt(parts[0]),
      start: parseTime(parts[1]),
      end: parseTime(parts[2]),
      style: parts[3].trim(),
      name: parts[4].trim(),
      marginL: parseInt(parts[5]),
      marginR: parseInt(parts[6]),
      marginV: parseInt(parts[7]),
      effect: parts[8].trim(),
      text: parts.slice(9).join(',').trim()
    }
  }
  
  function parseTime(timeStr: string): number {
    const parts = timeStr.trim().split(':')
    const hours = parseInt(parts[0])
    const minutes = parseInt(parts[1])
    const seconds = parseFloat(parts[2])
    return hours * 3600 + minutes * 60 + seconds
  }
  
  function updateSubtitles() {
    if (!videoElement) return
    
    const currentTime = videoElement.currentTime
    activeSubtitles = subtitles.filter(sub => 
      currentTime >= sub.start && currentTime <= sub.end
    )
  }
  
  function getSubtitleStyle(dialogue: AssDialogue) {
    const style = styles[dialogue.style] || {}
    return {
      fontSize: `${style.fontsize || 24}px`,
      color: convertAssColor(style.primaryColour),
      textShadow: `0 0 ${style.shadow || 2}px rgba(0,0,0,0.8)`,
      fontWeight: style.bold ? 'bold' : 'normal',
      fontStyle: style.italic ? 'italic' : 'normal',
      ...getAlignment(style.alignment || 2)
    }
  }
  
  function convertAssColor(assColor: string): string {
    if (!assColor || !assColor.startsWith('&H')) return '#FFFFFF'
    
    // ASS color format: &HAABBGGRR
    const hex = assColor.slice(2, 8)
    const b = hex.slice(0, 2)
    const g = hex.slice(2, 4) 
    const r = hex.slice(4, 6)
    
    return `#${r}${g}${b}`
  }
  
  function getAlignment(alignment: number) {
    const vertical = Math.floor((alignment - 1) / 3)
    const horizontal = (alignment - 1) % 3
    
    const style: any = {}
    
    // Horizontal alignment
    if (horizontal === 0) style.left = '0'
    else if (horizontal === 1) style.left = '50%', style.transform = 'translateX(-50%)'
    else style.right = '0'
    
    // Vertical alignment
    if (vertical === 0) style.bottom = '0'
    else if (vertical === 1) style.top = '50%', style.transform = (style.transform || '') + ' translateY(-50%)'
    else style.top = '0'
    
    return style
  }
  
  function stripAssOverrides(text: string): string {
    return text.replace(/\{[^}]*\}/g, '')
  }
  
  onDestroy(() => {
    if (videoElement) {
      videoElement.removeEventListener('timeupdate', updateSubtitles)
    }
  })
</script>

<div bind:this={subtitleContainer} class="absolute inset-0 pointer-events-none">
  {#each activeSubtitles as subtitle (subtitle.start + subtitle.text)}
    <div
      class="absolute p-2"
      style={Object.entries(getSubtitleStyle(subtitle))
        .map(([k, v]) => `${k}: ${v}`)
        .join('; ')}
    >
      {stripAssOverrides(subtitle.text)}
    </div>
  {/each}
</div>