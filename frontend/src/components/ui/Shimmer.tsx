import { memo, useMemo, type CSSProperties, type ElementType } from "react";
import "./Shimmer.css";

export interface ShimmerProps {
  children: string;
  as?: ElementType;
  className?: string;
  duration?: number;
  spread?: number;
}

const ShimmerComponent = ({
  children,
  as: Component = "span",
  className = "",
  duration = 2,
  spread = 2,
}: ShimmerProps) => {
  const dynamicSpread = useMemo(() => {
    return children.length * spread;
  }, [children, spread]);

  // @ts-ignore - CSS Custom Properties in React style
  const style = {
    "--shimmer-spread": `${dynamicSpread}px`,
    "--shimmer-duration": `${duration}s`,
  } as CSSProperties;

  return (
    <Component
      className={`shimmer-text ${className}`}
      style={style}
    >
      {children}
    </Component>
  );
};

export const Shimmer = memo(ShimmerComponent);
