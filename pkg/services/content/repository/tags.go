package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *ContentRepository) CreateTag(ctx context.Context, db db.Queryer, tag content.Tag) (err error) {
	const query = `INSERT INTO tags
				(id, created_at, updated_at, name, description, website_id)
			VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = db.Exec(ctx, query, tag.ID, tag.CreatedAt, tag.UpdatedAt, tag.Name, tag.Description, tag.WebsiteID)
	if err != nil {
		return fmt.Errorf("content.CreateTag: %w", err)
	}

	return nil
}

func (repo *ContentRepository) FindTagByID(ctx context.Context, db db.Queryer, tagID guid.GUID) (tag content.Tag, err error) {
	const query = "SELECT * FROM tags WHERE id = $1"

	err = db.Get(ctx, &tag, query, tagID)
	if err != nil {
		if err == sql.ErrNoRows {
			return tag, content.ErrTagNotFound
		} else {
			return tag, fmt.Errorf("content.FindTagByID: %w", err)
		}
	}

	return tag, nil
}

func (repo *ContentRepository) UpdateTag(ctx context.Context, db db.Queryer, tag content.Tag) (err error) {
	const query = `UPDATE tags
		SET updated_at = $1, name = $2, description = $3
		WHERE id = $4`

	_, err = db.Exec(ctx, query, tag.UpdatedAt, tag.Name, tag.Description,
		tag.ID)
	if err != nil {
		return fmt.Errorf("content.UpdateTag: %w", err)
	}

	return nil
}

func (repo *ContentRepository) DeleteTag(ctx context.Context, db db.Queryer, tagID guid.GUID) (err error) {
	const query = `DELETE FROM tags WHERE id = $1`

	_, err = db.Exec(ctx, query, tagID)
	if err != nil {
		return fmt.Errorf("content.DeleteTag: %w", err)
	}

	return nil
}

func (repo *ContentRepository) FindTagsForPage(ctx context.Context, db db.Queryer, pageID guid.GUID) (tags []content.Tag, err error) {
	tags = make([]content.Tag, 4)
	const query = `SELECT * FROM tags WHERE id = ANY (
			SELECT tag_id FROM pages_tags WHERE page_id =  $1
		)
		ORDER BY name
	`

	err = db.Select(ctx, &tags, query, pageID)
	if err != nil {
		return tags, fmt.Errorf("content.FindTagsForPage: %w", err)
	}

	return tags, nil
}

func (repo *ContentRepository) FindTagByName(ctx context.Context, db db.Queryer, websiteID guid.GUID, name string) (tag content.Tag, err error) {
	const query = "SELECT * FROM tags WHERE website_id = $1 AND name = $2"

	err = db.Get(ctx, &tag, query, websiteID, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return tag, content.ErrTagNotFound
		} else {
			return tag, fmt.Errorf("content.FindTagByName: %w", err)
		}
	}

	return tag, nil
}

func (repo *ContentRepository) FindTagsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (tags []content.Tag, err error) {
	tags = make([]content.Tag, 0)
	const query = `SELECT * FROM tags
		WHERE website_id = $1
		ORDER BY name
	`

	err = db.Select(ctx, &tags, query, websiteID)
	if err != nil {
		return tags, fmt.Errorf("content.FindTagsForWebsite: %w", err)
	}

	return tags, nil
}

func (repo *ContentRepository) CreateTagPageRelation(ctx context.Context, db db.Queryer, relation content.TagPageRelation) (err error) {
	const query = `INSERT INTO pages_tags
				(page_id, tag_id)
			VALUES ($1, $2)`

	_, err = db.Exec(ctx, query, relation.PageID, relation.TagID)
	if err != nil {
		return fmt.Errorf("content.CreateTagPageRelation: %w", err)
	}

	return nil
}

func (repo *ContentRepository) DeleteTagPageRelation(ctx context.Context, db db.Queryer, pageID, tagID guid.GUID) (err error) {
	const query = `DELETE FROM pages_tags WHERE page_id = $1 AND tag_id = $2`

	_, err = db.Exec(ctx, query, pageID, tagID)
	if err != nil {
		return fmt.Errorf("content.DeleteTagPageRelation: %w", err)
	}

	return nil
}
