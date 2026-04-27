//! CRUSH straw2 placement, adapted for a single host.
//!
//! Failure domain = disk. The topology is a flat list of `(disk_uuid, weight)`
//! per pool. The algorithm is the same straw2 used by the dataplane crate;
//! this module is the meta-side, single-host counterpart.

use ring::digest;

use crate::topology::PoolTopology;

#[derive(Debug, thiserror::Error)]
pub enum CrushError {
    #[error("not enough disks for replication factor: have {have}, need {need}")]
    InsufficientDisks { have: usize, need: usize },
}

/// Select `count` distinct disks for the given chunk key.
pub fn select(
    chunk_key: &str,
    count: usize,
    topology: &PoolTopology,
) -> Result<Vec<String>, CrushError> {
    if topology.disks.len() < count {
        return Err(CrushError::InsufficientDisks {
            have: topology.disks.len(),
            need: count,
        });
    }
    let mut candidates: Vec<&crate::topology::DiskCandidate> = topology.disks.iter().collect();
    let mut out = Vec::with_capacity(count);

    for replica in 0..count {
        let best = candidates
            .iter()
            .enumerate()
            .map(|(idx, d)| (idx, straw2_draw(chunk_key, &d.disk_uuid, replica, d.weight)))
            .max_by(|(_, a), (_, b)| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
        if let Some((best_idx, _)) = best {
            let chosen = candidates.remove(best_idx);
            out.push(chosen.disk_uuid.clone());
        }
    }
    Ok(out)
}

fn straw2_draw(chunk_key: &str, candidate_id: &str, replica: usize, weight: u64) -> f64 {
    let hash = crush_hash(chunk_key, candidate_id, replica);
    let draw = (hash as f64 + 0.5) / (u32::MAX as f64 + 1.0);
    draw.ln() / weight.max(1) as f64
}

fn crush_hash(chunk_key: &str, candidate_id: &str, replica: usize) -> u32 {
    let mut ctx = digest::Context::new(&digest::SHA256);
    ctx.update(chunk_key.as_bytes());
    ctx.update(b":");
    ctx.update(candidate_id.as_bytes());
    ctx.update(b":");
    let mut buf = [0u8; 20];
    let len = {
        use std::io::Write;
        let mut cursor = std::io::Cursor::new(&mut buf[..]);
        write!(cursor, "{}", replica).unwrap();
        cursor.position() as usize
    };
    ctx.update(&buf[..len]);
    let result = ctx.finish();
    let bytes = result.as_ref();
    u32::from_be_bytes([bytes[0], bytes[1], bytes[2], bytes[3]])
}

/// Build the chunk key from `(volume_uuid, chunk_index)`.
pub fn chunk_key(volume_uuid: &str, chunk_index: u32) -> String {
    format!("{volume_uuid}/{chunk_index}")
}

#[cfg(test)]
mod tests {
    use super::*;

    fn topo(n: usize) -> PoolTopology {
        let mut t = PoolTopology::new("pool");
        for i in 0..n {
            t.push(format!("disk-{i}"), 100);
        }
        t
    }

    #[test]
    fn select_returns_requested_count() {
        let t = topo(5);
        let p = select("k", 3, &t).unwrap();
        assert_eq!(p.len(), 3);
    }

    #[test]
    fn select_is_deterministic() {
        let t = topo(5);
        let a = select("k", 3, &t).unwrap();
        let b = select("k", 3, &t).unwrap();
        assert_eq!(a, b);
    }

    #[test]
    fn select_distinct_disks() {
        let t = topo(5);
        let p = select("k", 3, &t).unwrap();
        let mut seen = std::collections::HashSet::new();
        for d in &p {
            assert!(seen.insert(d.clone()), "duplicate disk: {d}");
        }
    }

    #[test]
    fn select_insufficient_disks_errors() {
        let t = topo(2);
        let e = select("k", 3, &t).unwrap_err();
        matches!(e, CrushError::InsufficientDisks { .. });
    }

    #[test]
    fn select_distributes() {
        let t = topo(4);
        let mut counts = std::collections::HashMap::<String, usize>::new();
        for i in 0..1000 {
            let p = select(&format!("k-{i}"), 1, &t).unwrap();
            *counts.entry(p[0].clone()).or_insert(0) += 1;
        }
        for c in counts.values() {
            assert!((150..=350).contains(c), "got {c}, expected 150-350");
        }
    }

    #[test]
    fn select_respects_weight() {
        let mut t = PoolTopology::new("pool");
        t.push("heavy", 900);
        t.push("light", 100);
        let mut heavy = 0;
        for i in 0..1000 {
            let p = select(&format!("k-{i}"), 1, &t).unwrap();
            if p[0] == "heavy" {
                heavy += 1;
            }
        }
        assert!(heavy > 800, "heavy got {heavy}");
    }
}
