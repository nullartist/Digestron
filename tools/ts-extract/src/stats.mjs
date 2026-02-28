// Weights for structuralConfidence formula (USG v0.1 spec).
export const RESOLVED_EDGE_WEIGHT = 0.7;
export const SYMBOL_COVERAGE_WEIGHT = 0.2;
export const DYNAMIC_PENALTY_WEIGHT = 0.1;

/**
 * Computes structuralConfidence per the USG v0.1 spec:
 *   0.7 * resolvedEdgeRatio + 0.2 * symbolCoverageRatio + 0.1 * (1 - dynamicRatio)
 */
export function computeStructuralConfidence(resolvedEdgeRatio, symbolCoverageRatio, dynamicRatio) {
  return (
    RESOLVED_EDGE_WEIGHT * resolvedEdgeRatio +
    SYMBOL_COVERAGE_WEIGHT * symbolCoverageRatio +
    DYNAMIC_PENALTY_WEIGHT * (1 - dynamicRatio)
  );
}
