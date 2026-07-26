export interface FullscreenTarget {
  requestFullscreen(): Promise<void>
}

export interface OrientationController {
  lock?(orientation: 'landscape'): Promise<void>
  unlock?(): void
  addEventListener?(type: 'change', listener: () => void): void
  removeEventListener?(type: 'change', listener: () => void): void
}

export async function requestViewerFullscreen(
  target: FullscreenTarget,
): Promise<void> {
  await target.requestFullscreen()
}

export async function lockViewerOrientation(orientation?: OrientationController): Promise<void> {
  try {
    await orientation?.lock?.('landscape')
  } catch {
    // Orientation lock is optional and commonly unavailable on desktop/iOS.
  }
}

export function createOrientationLockCoordinator(orientation?: OrientationController) {
  let desiredFullscreen = false
  let destroyed = false
  let locked = false
  let lockInFlight: Promise<void> | null = null

  function startLock() {
    if (destroyed || !desiredFullscreen || lockInFlight || locked) return
    lockInFlight = (async () => {
      await lockViewerOrientation(orientation)
      lockInFlight = null
      if (destroyed || !desiredFullscreen) {
        releaseViewerOrientation(orientation)
        return
      }
      locked = true
    })()
  }

  return {
    setFullscreen(active: boolean) {
      if (destroyed) return
      desiredFullscreen = active
      if (active) {
        startLock()
      } else if (locked) {
        locked = false
        releaseViewerOrientation(orientation)
      }
    },
    destroy() {
      if (destroyed) return
      destroyed = true
      desiredFullscreen = false
      if (locked) {
        locked = false
        releaseViewerOrientation(orientation)
      }
    },
    async waitForIdle() {
      await lockInFlight
    },
  }
}

export function releaseViewerOrientation(orientation?: OrientationController): void {
  try {
    orientation?.unlock?.()
  } catch {
    // Ignore browsers that expose unlock but reject the operation.
  }
}
