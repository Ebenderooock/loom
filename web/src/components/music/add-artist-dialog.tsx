import { TitleSearchDialog } from "@/components/titles/title-experience";

export function AddArtistDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <TitleSearchDialog media="artist" open={open} onOpenChange={onOpenChange} />
  );
}
