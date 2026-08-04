'use client';

import { useEffect, useMemo, useState } from 'react';
import dynamic from 'next/dynamic';
import { useTheme } from 'next-themes';
import Image from 'next/image';
import { cn } from '@/lib/cn';

const GrainGradient = dynamic(
  () => import('@paper-design/shaders-react').then((m) => m.GrainGradient),
  { ssr: false }
);

type ShaderProfile = {
  delayMs: number;
  idleTimeoutMs: number;
  softness: number;
  intensity: number;
  noise: number;
  reduceMotion: boolean;
};

const DEFAULT_SHADER_PROFILE: ShaderProfile = {
  delayMs: 500,
  idleTimeoutMs: 1200,
  softness: 1,
  intensity: 0.85,
  noise: 0.45,
  reduceMotion: false,
};

function detectShaderProfile(): ShaderProfile {
  if (typeof window === 'undefined') {
    // Must be consistent between SSR and the client's first render.
    return DEFAULT_SHADER_PROFILE;
  }

  const reduceMotion =
    window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches ?? false;

  const navAny = navigator as unknown as {
    deviceMemory?: number;
    hardwareConcurrency?: number;
  };

  const memory =
    typeof navAny.deviceMemory === 'number' ? navAny.deviceMemory : 8;
  const cores =
    typeof navAny.hardwareConcurrency === 'number'
      ? navAny.hardwareConcurrency
      : 8;
  const isSmallScreen =
    window.matchMedia?.('(max-width: 1023px)')?.matches ?? false;

  // Always render shaders, but adapt quality to reduce jank on low-end / small screens.
  const lowEnd = memory <= 4 || cores <= 4;

  if (reduceMotion) {
    return {
      delayMs: 350,
      idleTimeoutMs: 1200,
      softness: 1,
      intensity: 0.55,
      noise: 0.22,
      reduceMotion: true,
    };
  }

  if (lowEnd || isSmallScreen) {
    return {
      delayMs: 550,
      idleTimeoutMs: 1600,
      softness: 1,
      intensity: 0.7,
      noise: 0.28,
      reduceMotion: false,
    };
  }

  return {
    delayMs: 250,
    idleTimeoutMs: 1000,
    softness: 1,
    intensity: 0.9,
    noise: 0.5,
    reduceMotion: false,
  };
}

export function Hero() {
  const { resolvedTheme } = useTheme();
  const [showShaders, setShowShaders] = useState(false);
  const [logoReady, setLogoReady] = useState(false);
  const [mounted, setMounted] = useState(false);
  // Avoid hydration mismatch: the first client render must match SSR output.
  const [profile, setProfile] = useState<ShaderProfile>(DEFAULT_SHADER_PROFILE);

  // Blue Whale 蓝系 shader（去除原粉色 #EC4899 / #F9A8D4）
  const shaderColors = useMemo(
    () =>
      resolvedTheme === 'dark'
        ? ['#3B82F6', '#06B6D4', '#8B5CF6', '#1E3A8A00']
        : ['#60A5FA', '#22D3EE', '#A78BFA', '#DBEAFE20'],
    [resolvedTheme]
  );

  useEffect(() => {
    setMounted(true);
    setProfile(detectShaderProfile());
  }, []);

  useEffect(() => {
    if (!mounted) return;

    let cancelled = false;
    let idleId: number | undefined;
    const start = () => {
      if (!cancelled) setShowShaders(true);
    };

    const win = window as unknown as {
      requestIdleCallback?: (
        cb: () => void,
        options?: { timeout?: number }
      ) => number;
      cancelIdleCallback?: (id: number) => void;
    };

    // Always keep the effect, but start it after first paint/idle to minimize jank.
    const t = window.setTimeout(() => {
      if (cancelled) return;
      if (typeof win.requestIdleCallback === 'function') {
        idleId = win.requestIdleCallback(start, {
          timeout: profile.idleTimeoutMs,
        });
        return;
      }
      start();
    }, profile.delayMs);

    return () => {
      cancelled = true;
      window.clearTimeout(t);
      if (typeof idleId === 'number') {
        win.cancelIdleCallback?.(idleId);
      }
    };
  }, [mounted, profile.delayMs, profile.idleTimeoutMs]);

  return (
    <>
      {/* Lightweight fallback while shader bundle loads */}
      <div
        className="absolute inset-0"
        style={{
          background:
            resolvedTheme === 'dark'
              ? 'radial-gradient(1200px 800px at 20% 15%, rgba(59,130,246,.28), transparent 60%), radial-gradient(1000px 700px at 70% 25%, rgba(6,182,212,.24), transparent 55%), radial-gradient(900px 700px at 55% 70%, rgba(139,92,246,.18), transparent 60%)'
              : 'radial-gradient(1200px 800px at 20% 15%, rgba(96,165,250,.25), transparent 60%), radial-gradient(1000px 700px at 70% 25%, rgba(34,211,238,.22), transparent 55%), radial-gradient(900px 700px at 55% 70%, rgba(167,139,250,.18), transparent 60%)',
        }}
      />

      {showShaders && (
        <GrainGradient
          className={cn(
            'absolute inset-0 duration-800',
            profile.reduceMotion ? 'animate-none' : 'animate-fd-fade-in'
          )}
          colors={shaderColors}
          colorBack="#00000000"
          softness={profile.softness}
          intensity={profile.intensity}
          noise={profile.noise}
          shape="corners"
        />
      )}

      {/* Logo */}
      {mounted && (
        <div
          className={cn(
            'absolute hidden lg:top-[10%] lg:right-[10%] lg:block',
            logoReady
              ? 'animate-in fade-in zoom-in-95 duration-700'
              : 'invisible'
          )}
        >
          <Image
            src="https://bluewhale-assets.tos-cn-beijing.volces.com/blue-whale-api/blue_whale.svg"
            alt="Blue Whale API"
            width={288}
            height={288}
            className="size-32 drop-shadow-2xl sm:size-40 md:size-56 lg:size-72"
            onLoad={() => setLogoReady(true)}
            priority
            unoptimized
          />
        </div>
      )}
    </>
  );
}
