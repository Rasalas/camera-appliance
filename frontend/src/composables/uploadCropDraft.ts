import { createDraftAutosave, type SaveState } from './draftAutosave.ts'
import type { UploadCrop } from '../types/index.ts'

export function validUploadCrop(crop: UploadCrop): boolean {
  return !crop.enabled || ([crop.x, crop.y, crop.width, crop.height].every(Number.isFinite)
    && crop.x >= 0 && crop.y >= 0 && crop.width > 0 && crop.height > 0
    && crop.x + crop.width <= 100 && crop.y + crop.height <= 100)
}

export function createCropAutosave(save: (crop: UploadCrop) => Promise<unknown>, report: (state: SaveState, error?: unknown) => void, delay = 450) {
  return createDraftAutosave(save, report, crop => ({ ...crop }), validUploadCrop, delay)
}
