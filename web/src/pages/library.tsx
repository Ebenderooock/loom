import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useSetPageHeader } from "@/hooks/use-page-header";

export function LibraryPage() {
  useSetPageHeader("Library");
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-6">
        {Array.from({ length: 12 }).map((_, i) => (
          <Card key={i} className="overflow-hidden">
            <Skeleton className="aspect-[2/3] w-full rounded-none" />
            <CardHeader className="p-3">
              <CardTitle className="text-sm">
                <Skeleton className="h-3 w-3/4" />
              </CardTitle>
            </CardHeader>
            <CardContent className="p-3 pt-0">
              <Skeleton className="h-3 w-1/2" />
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
