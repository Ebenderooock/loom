import * as React from "react";

interface PageHeaderState {
  title: string;
  subtitle?: string;
}

const PageHeaderContext = React.createContext<{
  header: PageHeaderState;
  setHeader: (h: PageHeaderState) => void;
}>({
  header: { title: "" },
  setHeader: () => {},
});

export function PageHeaderProvider({ children }: { children: React.ReactNode }) {
  const [header, setHeader] = React.useState<PageHeaderState>({ title: "" });
  const value = React.useMemo(() => ({ header, setHeader }), [header]);
  return (
    <PageHeaderContext.Provider value={value}>
      {children}
    </PageHeaderContext.Provider>
  );
}

export function usePageHeader() {
  return React.useContext(PageHeaderContext);
}

/** Call in a page component to set the header title/subtitle. */
export function useSetPageHeader(title: string, subtitle?: string) {
  const { setHeader } = usePageHeader();
  React.useEffect(() => {
    setHeader({ title, subtitle });
  }, [title, subtitle, setHeader]);
}
