import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

function PlaceholderTable({ caption }: { caption: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{caption}</CardTitle>
      </CardHeader>
      <CardContent>
        <table className="w-full text-sm">
          <caption className="sr-only">{caption}</caption>
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th scope="col" className="py-2">
                Title
              </th>
              <th scope="col" className="py-2">
                Status
              </th>
              <th scope="col" className="py-2">
                Updated
              </th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td
                colSpan={3}
                className="py-6 text-center text-muted-foreground"
              >
                Nothing to show yet.
              </td>
            </tr>
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}

export function ActivityPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Activity</h1>
        <p className="text-sm text-muted-foreground">
          What is happening across your indexers and download clients.
        </p>
      </div>
      <Tabs defaultValue="queue">
        <TabsList>
          <TabsTrigger value="queue">Queue</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="blocklist">Blocklist</TabsTrigger>
        </TabsList>
        <TabsContent value="queue">
          <PlaceholderTable caption="Active queue" />
        </TabsContent>
        <TabsContent value="history">
          <PlaceholderTable caption="Recent history" />
        </TabsContent>
        <TabsContent value="blocklist">
          <PlaceholderTable caption="Blocklisted releases" />
        </TabsContent>
      </Tabs>
    </div>
  );
}
