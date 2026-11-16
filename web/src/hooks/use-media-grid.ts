import { useSyncExternalStore } from "react";

const STORAGE_KEY = "loom-media-card-size";
const CHANGE_EVENT = "loom-media-card-size-change";

export type MediaCardSize = "small" | "medium" | "large";

const gridClasses: Record<MediaCardSize, string> = {
  small:
    "grid grid-cols-2 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-8 2xl:grid-cols-10 min-[1921px]:grid-cols-[repeat(auto-fill,9rem)]",
  medium:
    "grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8 min-[1921px]:grid-cols-[repeat(auto-fill,11.5rem)]",
  large:
    "grid grid-cols-2 gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 min-[1921px]:grid-cols-[repeat(auto-fill,15.5rem)]",
};

function getSize(): MediaCardSize {
  const value = localStorage.getItem(STORAGE_KEY);
  return value === "small" || value === "large" ? value : "medium";
}

function subscribe(callback: () => void) {
  window.addEventListener(CHANGE_EVENT, callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener(CHANGE_EVENT, callback);
    window.removeEventListener("storage", callback);
  };
}

export function useMediaCardSize() {
  return useSyncExternalStore<MediaCardSize>(
    subscribe,
    getSize,
    () => "medium",
  );
}

export function setMediaCardSize(size: MediaCardSize) {
  localStorage.setItem(STORAGE_KEY, size);
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

export function useMediaGridClass() {
  return gridClasses[useMediaCardSize()];
}
