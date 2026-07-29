// useInfiniteScroll: triggers a callback when a sentinel element enters the viewport.
// Uses IntersectionObserver for efficient scroll detection without event listeners.

import { useCallback, useEffect, useRef, useState } from "react";

interface UseInfiniteScrollOptions {
  /** Whether there are more items to load. */
  hasMore: boolean;
  /** Whether data is currently being loaded. */
  loading: boolean;
  /** Callback to load more data. */
  onLoadMore: () => void;
  /** Distance from the sentinel (in pixels) to trigger the load. Default: 200. */
  rootMargin?: number;
}

/**
 * Returns a ref to attach to a sentinel element at the bottom of a scrollable list.
 * When the sentinel enters the viewport (with margin), `onLoadMore` is called.
 */
export function useInfiniteScroll({
  hasMore,
  loading,
  onLoadMore,
  rootMargin = 200,
}: UseInfiniteScrollOptions) {
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const [observerEnabled, setObserverEnabled] = useState(true);

  const handleIntersect = useCallback(
    (entries: IntersectionObserverEntry[]) => {
      const entry = entries[0];
      if (entry.isIntersecting && hasMore && !loading && observerEnabled) {
        onLoadMore();
      }
    },
    [hasMore, loading, onLoadMore, observerEnabled],
  );

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(handleIntersect, {
      rootMargin: `${rootMargin}px`,
      threshold: 0,
    });

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [handleIntersect, rootMargin]);

  // Pause observer briefly after a load to prevent rapid-fire triggers
  useEffect(() => {
    if (loading) {
      setObserverEnabled(false);
    } else {
      // Small delay after loading completes to avoid immediate re-trigger
      const timer = setTimeout(() => setObserverEnabled(true), 150);
      return () => clearTimeout(timer);
    }
  }, [loading]);

  return sentinelRef;
}
