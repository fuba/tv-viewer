// EPG (Electronic Program Guide) types

export interface EPGProgram {
  id: number
  eventId: number
  serviceId: number
  networkId: number
  startAt: number      // Unix milliseconds
  duration: number     // milliseconds
  name: string
  description: string
  genre: {
    lv1: number
    lv2: number
  }
}

export interface EPGChannel {
  channel: string
  type: 'GR' | 'BS' | 'CS'
  serviceId: number
  name: string
  programs: EPGProgram[]
}

export interface EPGResponse {
  channels: EPGChannel[]
  timeRange: {
    from: number
    to: number
  }
}
