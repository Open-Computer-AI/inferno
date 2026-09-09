import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'

/** Composite route targets are concrete account platforms, never group-only options. */
export const buildCompositeRoutePlatformOptions = () => [...CONCRETE_PLATFORM_OPTIONS]
