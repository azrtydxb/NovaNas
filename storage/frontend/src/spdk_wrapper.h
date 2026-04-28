/* SPDK header wrapper for bindgen — frontend variant.
 *
 * The frontend daemon hosts the NVMe-oF target; it does NOT need lvol /
 * blobstore / NVMe initiator headers. Keep this surface narrow.
 */
#include <spdk/stdinc.h>
#include <spdk/env.h>
#include <spdk/event.h>
#include <spdk/bdev.h>
#include <spdk/bdev_module.h>
#include <spdk/nvmf.h>
#include <spdk/nvmf_spec.h>
#include <spdk/nvmf_transport.h>
#include <spdk/json.h>
#include <spdk/jsonrpc.h>
#include <spdk/rpc.h>
#include <spdk/thread.h>
#include <spdk/log.h>
#include <spdk/string.h>
#include <spdk/sock.h>

/* Forward declarations for SPDK malloc bdev module internals (used by
 * the frontend's bdev_manager bring-up helpers). */
struct malloc_bdev_opts {
    char *name;
    struct spdk_uuid uuid;
    uint64_t num_blocks;
    uint32_t block_size;
    uint32_t physical_block_size;
    uint32_t optimal_io_boundary;
    uint32_t md_size;
    bool md_interleave;
    enum spdk_dif_type dif_type;
    bool dif_is_head_of_md;
    enum spdk_dif_pi_format dif_pi_format;
    int32_t numa_id;
};

int create_malloc_disk(struct spdk_bdev **bdev, const struct malloc_bdev_opts *opts);
void delete_malloc_disk(struct spdk_bdev *bdev, spdk_bdev_unregister_cb cb_fn, void *cb_arg);

/* Thin C wrapper around SPDK's create_uring_bdev (defined in
 * uring_wrapper.c). */
struct spdk_bdev *novanas_create_uring_bdev(const char *name,
                                              const char *filename,
                                              uint32_t block_size);
