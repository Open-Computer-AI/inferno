import { onBeforeUnmount, ref, type Ref } from 'vue'

export interface SpringConfig {
  stiffness: number
  damping: number
  mass: number
}

/**
 * The reference decks are built on Framer Motion's `useSpring`. We are on Vue
 * with @vueuse/core and no Motion, so this is the same physics written out:
 * a damped harmonic oscillator integrated per frame.
 *
 *     a = (-k * (x - target) - c * v) / m
 *
 * WHY A SPRING AND NOT A TWEEN. A tween has a fixed duration, so retargeting
 * it mid-flight either restarts the clock (the number visibly stutters) or
 * finishes the old journey first (it lags). A spring has no duration to
 * restart -- it carries its current velocity into the new target, which is
 * exactly the "retargets from wherever it sits" behaviour every one of the
 * three specs asks for, on the hero numbers and on the scrubber alike.
 *
 * dt is clamped to 64ms. A backgrounded tab resumes with a multi-second delta,
 * and feeding that raw into the integrator makes the spring explode: the
 * position overshoots by orders of magnitude and then oscillates back, which
 * reads as the number briefly showing garbage. Clamping costs nothing because
 * a settled spring is idle anyway.
 */
export function useSpringValue(
  initial: number,
  config: SpringConfig = { stiffness: 190, damping: 27, mass: 0.7 }
): {
  value: Ref<number>
  set: (next: number) => void
  jump: (next: number) => void
} {
  const value = ref(initial)
  let target = initial
  let velocity = 0
  let frame: number | null = null
  let last = 0

  const reduced =
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches

  function stop() {
    if (frame !== null) {
      cancelAnimationFrame(frame)
      frame = null
    }
  }

  function tick(now: number) {
    const dt = Math.min((now - last) / 1000, 0.064)
    last = now

    const displacement = value.value - target
    const acceleration = (-config.stiffness * displacement - config.damping * velocity) / config.mass
    velocity += acceleration * dt
    value.value += velocity * dt

    /* Settle on BOTH position and velocity. Position alone stops the spring as
       it flies through the target at speed, which freezes it a hair off and
       leaves the last digit wrong. */
    if (Math.abs(value.value - target) < 0.01 && Math.abs(velocity) < 0.01) {
      value.value = target
      velocity = 0
      frame = null
      return
    }
    frame = requestAnimationFrame(tick)
  }

  function start() {
    if (frame !== null) return
    last = performance.now()
    frame = requestAnimationFrame(tick)
  }

  function jump(next: number) {
    stop()
    target = next
    velocity = 0
    value.value = next
  }

  function set(next: number) {
    if (next === target) return
    target = next
    if (reduced) {
      jump(next)
      return
    }
    start()
  }

  onBeforeUnmount(stop)

  return { value, set, jump }
}
