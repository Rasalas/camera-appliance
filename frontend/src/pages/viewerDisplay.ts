import type { CameraDisplay } from '../types'

// Measure the frame wrapper rather than using container query units. Older
// Safari versions otherwise discard the dimensions of quarter-turn frames.
export function frameDisplayStyle(display: CameraDisplay, size?: { width: number; height: number }) {
  const crop = display.crop
  const rotated = display.rotation === 90 || display.rotation === 270
  return {
    left: rotated ? '50%' : `${-crop.x / crop.width * 100}%`,
    top: rotated ? '50%' : `${-crop.y / crop.height * 100}%`,
    width: rotated && size ? `${size.height * 100 / crop.width}px` : `${10000 / crop.width}%`,
    height: rotated && size ? `${size.width * 100 / crop.height}px` : `${10000 / crop.height}%`,
    transform: `${rotated ? 'translate(-50%, -50%) ' : ''}rotate(${display.rotation}deg) scaleX(${display.mirror ? -1 : 1}) scaleY(${display.flip ? -1 : 1})${rotated ? ` translate(${50 - crop.x - crop.width / 2}%, ${50 - crop.y - crop.height / 2}%)` : ''}`,
    '--display-fit': display.fit_mode
  }
}
