import { useCustomTiers } from "@/lib/custom-formats-api";

export function QualityLabel({ quality }: { quality: string }) {
  if (!quality.startsWith("custom-tier:")) return <>{quality}</>;
  return <CustomTierLabel id={quality.slice("custom-tier:".length)} />;
}

function CustomTierLabel({ id }: { id: string }) {
  const tiers = useCustomTiers();
  if (tiers.isPending) return <>Loading tier...</>;
  if (tiers.isError) {
    return (
      <span role="alert" title={tiers.error.message}>
        Custom tier unavailable
      </span>
    );
  }
  const tier = tiers.data?.find((item) => item.id === id);
  if (!tier) {
    return (
      <span role="alert" title={`Custom tier ${id} was not found`}>
        Missing custom tier
      </span>
    );
  }
  return <>{tier.name}</>;
}
