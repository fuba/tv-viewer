export async function reserveProgram(programId: number): Promise<void> {
  if (!Number.isSafeInteger(programId) || programId <= 0) {
    throw new Error('番組IDが不正です')
  }
  const response = await fetch('/api/recordings/reserve', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ programId })
  })
  if (response.ok) return

  let message = '録画予約に失敗しました'
  try {
    const body = await response.json()
    if (typeof body.error === 'string' && body.error) message = body.error
  } catch {
    // Keep the user-facing fallback for non-JSON upstream failures.
  }
  throw new Error(message)
}
