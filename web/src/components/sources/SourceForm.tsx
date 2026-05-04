import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import type { UserSource, UserSourceCreate, SourceType } from "@/lib/sources-api";

const RSSConfigSchema = z.object({
  url: z.string().url("Must be a valid URL"),
  refresh_interval_minutes: z.coerce.number().min(15).default(60),
});

const ScraperConfigSchema = z.object({
  url: z.string().url("Must be a valid URL"),
  selector_type: z.enum(["css", "xpath"]),
  item_selector: z.string().min(1, "Item selector is required"),
  title_selector: z.string().min(1, "Title selector is required"),
  link_selector: z.string().optional(),
  published_selector: z.string().optional(),
  auth_type: z.enum(["none", "basic", "apikey"]).default("none"),
  username: z.string().optional(),
  password: z.string().optional(),
  api_key: z.string().optional(),
  refresh_interval_minutes: z.coerce.number().min(15).default(60),
});

const FormSchema = z.object({
  name: z.string().min(1, "Name is required"),
  type: z.enum(["rss", "scraper"]),
  enabled: z.boolean().default(true),
  config: z.union([RSSConfigSchema, ScraperConfigSchema]),
});

type FormValues = z.infer<typeof FormSchema>;

interface SourceFormProps {
  open: boolean;
  source?: UserSource;
  onClose: () => void;
  onSave: (data: UserSourceCreate | { id: string; patch: Partial<FormValues> }) => void;
  isLoading?: boolean;
}

export function SourceForm({ open, source, onClose, onSave, isLoading }: SourceFormProps) {
  const [sourceType, setSourceType] = useState<SourceType>(source?.type ?? "rss");

  const form = useForm<FormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: source
      ? {
          name: source.name,
          type: source.type,
          enabled: source.enabled,
          config: source.config as any,
        }
      : {
          name: "",
          type: "rss",
          enabled: true,
          config: { url: "", refresh_interval_minutes: 60 },
        },
  });

  useEffect(() => {
    if (source) {
      setSourceType(source.type);
      form.reset({
        name: source.name,
        type: source.type,
        enabled: source.enabled,
        config: source.config as any,
      });
    } else {
      form.reset({
        name: "",
        type: "rss",
        enabled: true,
        config: { url: "", refresh_interval_minutes: 60 },
      });
    }
  }, [source, open, form]);

  const onSubmit = (data: FormValues) => {
    if (source) {
      onSave({
        id: source.id,
        patch: {
          name: data.name,
          enabled: data.enabled,
          config: data.config,
        },
      });
    } else {
      onSave({
        name: data.name,
        type: data.type,
        enabled: data.enabled,
        config: data.config,
      });
    }
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{source ? "Edit Source" : "Add New Source"}</DialogTitle>
          <DialogDescription>
            {source
              ? "Update the source configuration"
              : "Create a new RSS feed or web scraper source"}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Source Name</FormLabel>
                <FormControl>
                  <Input placeholder="e.g., My RSS Feed" {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="type"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Source Type</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={(val) => {
                    field.onChange(val);
                    setSourceType(val as SourceType);
                  }}
                  disabled={!!source}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="rss">RSS Feed</SelectItem>
                    <SelectItem value="scraper">Web Scraper</SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="enabled"
            render={({ field }) => (
              <FormItem className="flex items-center gap-2">
                <FormControl>
                  <Checkbox checked={field.value} onCheckedChange={field.onChange} />
                </FormControl>
                <FormLabel className="mb-0 cursor-pointer">Enable this source</FormLabel>
              </FormItem>
            )}
          />

          {sourceType === "rss" && (
            <>
              <FormField
                control={form.control}
                name="config"
                render={() => (
                  <FormItem>
                    <FormLabel>RSS Feed URL</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="https://example.com/feed.xml"
                        {...form.register("config.url")}
                      />
                    </FormControl>
                    <FormMessage>{form.formState.errors.config?.url?.message}</FormMessage>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="config"
                render={() => (
                  <FormItem>
                    <FormLabel>Refresh Interval (minutes)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min="15"
                        placeholder="60"
                        {...form.register("config.refresh_interval_minutes")}
                      />
                    </FormControl>
                    <FormDescription>Minimum 15 minutes</FormDescription>
                    <FormMessage>
                      {form.formState.errors.config?.refresh_interval_minutes?.message}
                    </FormMessage>
                  </FormItem>
                )}
              />
            </>
          )}

          {sourceType === "scraper" && (
            <>
              <FormField
                control={form.control}
                name="config"
                render={() => (
                  <FormItem>
                    <FormLabel>Website URL</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="https://example.com/releases"
                        {...form.register("config.url")}
                      />
                    </FormControl>
                    <FormMessage>{form.formState.errors.config?.url?.message}</FormMessage>
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="config"
                  render={() => (
                    <FormItem>
                      <FormLabel>Selector Type</FormLabel>
                      <Select
                        value={(form.getValues().config as any)?.selector_type ?? "css"}
                        onValueChange={(val) =>
                          form.setValue("config.selector_type" as any, val)
                        }
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="css">CSS</SelectItem>
                          <SelectItem value="xpath">XPath</SelectItem>
                        </SelectContent>
                      </Select>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="config"
                  render={() => (
                    <FormItem>
                      <FormLabel>Auth Type</FormLabel>
                      <Select
                        value={(form.getValues().config as any)?.auth_type ?? "none"}
                        onValueChange={(val) => form.setValue("config.auth_type" as any, val)}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="none">None</SelectItem>
                          <SelectItem value="basic">Basic Auth</SelectItem>
                          <SelectItem value="apikey">API Key</SelectItem>
                        </SelectContent>
                      </Select>
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="config"
                render={() => (
                  <FormItem>
                    <FormLabel>Item Selector *</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="div.release"
                        {...form.register("config.item_selector")}
                      />
                    </FormControl>
                    <FormDescription>CSS or XPath to each item container</FormDescription>
                    <FormMessage>
                      {(form.formState.errors.config as any)?.item_selector?.message}
                    </FormMessage>
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="config"
                render={() => (
                  <FormItem>
                    <FormLabel>Title Selector *</FormLabel>
                    <FormControl>
                      <Input placeholder="h2" {...form.register("config.title_selector")} />
                    </FormControl>
                    <FormDescription>CSS or XPath to the title within each item</FormDescription>
                    <FormMessage>
                      {(form.formState.errors.config as any)?.title_selector?.message}
                    </FormMessage>
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="config"
                  render={() => (
                    <FormItem>
                      <FormLabel>Link Selector</FormLabel>
                      <FormControl>
                        <Input placeholder="a" {...form.register("config.link_selector")} />
                      </FormControl>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="config"
                  render={() => (
                    <FormItem>
                      <FormLabel>Published Selector</FormLabel>
                      <FormControl>
                        <Input placeholder=".date" {...form.register("config.published_selector")} />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="config"
                render={() => (
                  <FormItem>
                    <FormLabel>Refresh Interval (minutes)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min="15"
                        placeholder="60"
                        {...form.register("config.refresh_interval_minutes")}
                      />
                    </FormControl>
                    <FormDescription>Minimum 15 minutes</FormDescription>
                  </FormItem>
                )}
              />
            </>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading ? "Saving..." : source ? "Update" : "Create"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
