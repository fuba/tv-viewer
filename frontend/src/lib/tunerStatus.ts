export interface TunerInfo {
  type: string
  total: number
  using: number
  free: number
  viewerUsing: number
  epgUsing: number
  otherUsing: number
  viewerShared: number
}

export function tunerDetails(tuner: TunerInfo): string {
  const shared = tuner.viewerShared > 0 ? `（うち共有${tuner.viewerShared}台）` : ''
  return `TV Viewer ${tuner.viewerUsing}台${shared} / EPG取得 ${tuner.epgUsing}台 / その他 ${tuner.otherUsing}台 / 空き ${tuner.free}台`
}
